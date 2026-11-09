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
		return errors.New("pull: 元数据获取需要 Pool")
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
			return call(tdx.WithContext(ctx, c))
		}
		err := s.cfg.Pool.DoContext(ctx, fn)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}
		if !retryable(err) || attempt+1 == max(1, s.retry) {
			return err
		}
		if err := s.retryWait(ctx, attempt); err != nil {
			return err
		}
	}
	return nil
}

// FetchCodes fetches and validates a standard-market snapshot without persistence.
// On error it returns a zero snapshot; callers decide whether to save or publish a successful result.
func (s *Service) FetchCodes(ctx context.Context, exchanges ...protocol.Exchange) (model.CodeSnapshot, error) {
	if len(exchanges) == 0 {
		exchanges = []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ}
	}
	out := model.CodeSnapshot{Exchanges: append([]protocol.Exchange(nil), exchanges...), FetchedAt: time.Now().UTC()}
	if err := out.Validate(); err != nil {
		return model.CodeSnapshot{}, err
	}
	for _, ex := range exchanges {
		var response *tdx.CodeResp
		err := s.metadataCall(ctx, func(c tdx.Client) error {
			var err error
			response, err = c.GetCodeAll(ex)
			return err
		})
		if err != nil {
			return model.CodeSnapshot{}, err
		}
		if response == nil || response.Count != len(response.List) {
			return model.CodeSnapshot{}, fmt.Errorf("pull: incomplete code snapshot for %s", ex)
		}
		for _, item := range response.List {
			row := item.Standard
			if row == nil {
				return model.CodeSnapshot{}, fmt.Errorf("pull: standard code metadata required")
			}
			out.Rows = append(out.Rows, &model.CodeModel{Exchange: ex.String(), Code: row.Code, Name: row.Name, Multiple: row.Multiple, Decimal: row.Decimal, LastPrice: row.LastPrice})
		}
	}
	if err := out.Validate(); err != nil {
		return model.CodeSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.CodeSnapshot{}, err
	}
	out.FetchedAt = time.Now().UTC()
	return out, nil
}

// FetchGbbq fetches complete histories for the requested securities without persistence.
// Errors return a zero snapshot, never partial event histories.
func (s *Service) FetchGbbq(ctx context.Context, inputs []string) (model.GbbqSnapshot, error) {
	out := model.GbbqSnapshot{FetchedAt: time.Now().UTC()}
	seen := map[string]bool{}
	for _, input := range inputs {
		key, err := model.StandardCode(input)
		if err != nil {
			return model.GbbqSnapshot{}, err
		}
		if !seen[key] {
			out.Codes = append(out.Codes, key)
			seen[key] = true
		}
	}
	sort.Strings(out.Codes)
	if err := out.Validate(); err != nil {
		return model.GbbqSnapshot{}, err
	}
	for _, code := range out.Codes {
		var response *protocol.GbbqResp
		err := s.metadataCall(ctx, func(c tdx.Client) error { var err error; response, err = c.GetGbbq(code); return err })
		if err != nil {
			return model.GbbqSnapshot{}, err
		}
		if response == nil || int(response.Count) != len(response.List) {
			return model.GbbqSnapshot{}, fmt.Errorf("pull: incomplete events for %s", code)
		}
		for _, row := range response.List {
			if row == nil {
				return model.GbbqSnapshot{}, fmt.Errorf("pull: nil equity event")
			}
			copy := *row
			key, err := model.StandardCode(copy.Code)
			if err != nil || key != code {
				return model.GbbqSnapshot{}, fmt.Errorf("pull: event does not belong to %s", code)
			}
			copy.Code = key
			out.Rows = append(out.Rows, &copy)
		}
	}
	if err := out.Validate(); err != nil {
		return model.GbbqSnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.GbbqSnapshot{}, err
	}
	out.FetchedAt = time.Now().UTC()
	return out, nil
}

// FetchWorkdays derives observed mainland trading dates from Shanghai-index daily bars.
// It never persists data. An empty or invalid response returns a zero snapshot and an error.
func (s *Service) FetchWorkdays(ctx context.Context) (model.WorkdaySnapshot, error) {
	out := model.WorkdaySnapshot{Market: protocol.MarketAStock, Source: "tdx:sh000001", FetchedAt: time.Now().UTC()}
	var response *tdx.KlineResp
	err := s.metadataCall(ctx, func(c tdx.Client) error { var err error; response, err = c.GetKlineDayAll("sh000001"); return err })
	if err != nil {
		return model.WorkdaySnapshot{}, err
	}
	if response == nil || response.Count != len(response.List) {
		return model.WorkdaySnapshot{}, errors.New("pull: incomplete calendar response")
	}
	seen := map[string]bool{}
	for _, row := range response.List {
		if err := model.ValidateSourceTime(row.Time, time.DateOnly); err != nil {
			return model.WorkdaySnapshot{}, err
		}
		if !seen[row.Time] {
			out.Dates = append(out.Dates, row.Time)
			seen[row.Time] = true
		}
	}
	sort.Strings(out.Dates)
	if len(out.Dates) == 0 {
		return model.WorkdaySnapshot{}, errors.New("pull: empty calendar response cannot certify coverage")
	}
	out.From, out.Through = out.Dates[0], out.Dates[len(out.Dates)-1]
	if err := out.Validate(); err != nil {
		return model.WorkdaySnapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return model.WorkdaySnapshot{}, err
	}
	out.FetchedAt = time.Now().UTC()
	return out, nil
}
