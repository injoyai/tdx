package pull

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// metadataCall checks cancellation between bounded client operations. The caller owns
// client timeouts; cancellation never closes a client borrowed from a shared pool.
func (s *Service) metadataCall(ctx context.Context, call func(tdx.Client) error) error {
	if ctx == nil {
		return errors.New("pull: nil context")
	}
	if s.closed.Load() {
		return net.ErrClosed
	}
	if isNil(s.cfg.Pool) {
		return errors.New("pull: 元数据同步需要 Pool")
	}
	for attempt := 0; attempt < max(1, s.retry); attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		fn := func(c tdx.Client) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if isNil(c) {
				return errors.New("pull: pool returned a nil client")
			}
			return call(c)
		}
		var err error
		if pool, ok := s.cfg.Pool.(interface {
			DoContext(context.Context, func(tdx.Client) error) error
		}); ok {
			err = pool.DoContext(ctx, fn)
		} else {
			err = s.cfg.Pool.Do(fn)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}
		if attempt+1 == max(1, s.retry) {
			return err
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// SyncCodes fetches a complete standard-market snapshot before committing any exchange.
// It always performs the explicitly requested refresh and starts no recurring job.
func (s *Service) SyncCodes(ctx context.Context, exchanges ...protocol.Exchange) (model.CodeSnapshot, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	if len(exchanges) == 0 {
		exchanges = []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ}
	}
	out := model.CodeSnapshot{Exchanges: append([]protocol.Exchange(nil), exchanges...), FetchedAt: time.Now().UTC()}
	if isNil(s.cfg.CodesStore) {
		return out, errors.New("pull: 需要 CodesStore")
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	for _, ex := range exchanges {
		var response *tdx.CodeResp
		err := s.metadataCall(ctx, func(c tdx.Client) error {
			var err error
			response, err = c.GetCodeAll(ex)
			return err
		})
		if err != nil {
			return out, err
		}
		if response == nil || response.Count != len(response.List) {
			return out, fmt.Errorf("pull: incomplete code snapshot for %s", ex)
		}
		for _, item := range response.List {
			row := item.Standard
			if row == nil {
				return out, fmt.Errorf("pull: standard code metadata required")
			}
			out.Rows = append(out.Rows, &model.CodeModel{Exchange: ex.String(), Code: row.Code, Name: row.Name, Multiple: row.Multiple, Decimal: row.Decimal, LastPrice: row.LastPrice})
		}
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	out.FetchedAt = time.Now().UTC()
	return out, s.cfg.CodesStore.ReplaceCodes(ctx, out)
}

// SyncGbbq replaces histories only for requested securities, retaining delisted/unrelated histories.
func (s *Service) SyncGbbq(ctx context.Context, inputs []string) (model.GbbqSnapshot, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	out := model.GbbqSnapshot{FetchedAt: time.Now().UTC()}
	if isNil(s.cfg.GbbqStore) {
		return out, errors.New("pull: 需要 GbbqStore")
	}
	seen := map[string]bool{}
	for _, input := range inputs {
		key, err := model.StandardCode(input)
		if err != nil {
			return out, err
		}
		if !seen[key] {
			out.Codes = append(out.Codes, key)
			seen[key] = true
		}
	}
	sort.Strings(out.Codes)
	if err := out.Validate(); err != nil {
		return out, err
	}
	for _, code := range out.Codes {
		var response *protocol.GbbqResp
		err := s.metadataCall(ctx, func(c tdx.Client) error { var err error; response, err = c.GetGbbq(code); return err })
		if err != nil {
			return out, err
		}
		if response == nil || int(response.Count) != len(response.List) {
			return out, fmt.Errorf("pull: incomplete events for %s", code)
		}
		for _, row := range response.List {
			if row == nil {
				return out, fmt.Errorf("pull: nil equity event")
			}
			copy := *row
			key, err := model.StandardCode(copy.Code)
			if err != nil || key != code {
				return out, fmt.Errorf("pull: event does not belong to %s", code)
			}
			copy.Code = key
			out.Rows = append(out.Rows, &copy)
		}
	}
	if err := out.Validate(); err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	out.FetchedAt = time.Now().UTC()
	return out, s.cfg.GbbqStore.ReplaceGbbq(ctx, out)
}

// SyncWorkdays derives observed mainland trading dates from Shanghai-index daily bars.
// An empty response does not certify coverage or remove existing calendar data.
func (s *Service) SyncWorkdays(ctx context.Context) (model.WorkdaySnapshot, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	out := model.WorkdaySnapshot{Market: protocol.MarketAStock, Source: "tdx:sh000001", FetchedAt: time.Now().UTC()}
	if isNil(s.cfg.WorkdayStore) {
		return out, errors.New("pull: 需要 WorkdayStore")
	}
	var response *tdx.KlineResp
	err := s.metadataCall(ctx, func(c tdx.Client) error { var err error; response, err = c.GetKlineDayAll("sh000001"); return err })
	if err != nil {
		return out, err
	}
	if response == nil || response.Count != len(response.List) {
		return out, errors.New("pull: incomplete calendar response")
	}
	seen := map[string]bool{}
	for _, row := range response.List {
		if err := model.ValidateSourceTime(row.Time, time.DateOnly); err != nil {
			return out, err
		}
		if !seen[row.Time] {
			out.Dates = append(out.Dates, row.Time)
			seen[row.Time] = true
		}
	}
	sort.Strings(out.Dates)
	if len(out.Dates) == 0 {
		return out, errors.New("pull: empty calendar response cannot certify coverage")
	}
	out.From, out.Through = out.Dates[0], out.Dates[len(out.Dates)-1]
	if err := out.Validate(); err != nil {
		return out, err
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	out.FetchedAt = time.Now().UTC()
	return out, s.cfg.WorkdayStore.SaveWorkdays(ctx, out)
}
