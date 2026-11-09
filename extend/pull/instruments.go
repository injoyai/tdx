package pull

import (
	"context"
	"fmt"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// These states belong to one Update, so a late request cannot publish into a later round.
type instrumentDiscovery struct {
	items   []protocol.ExInstrument
	pending *instrumentRequest
}

type instrumentRequest struct {
	done  chan struct{}
	items []protocol.ExInstrument
	err   error
}

// GetInstruments Update 期间共用一次成功发现；Update 之外每次重新查询。
func (s *Service) GetInstruments() ([]protocol.ExInstrument, error) {
	s.discoveryMu.Lock()
	// Capture the context while the round is fixed, so a caller cannot pair an
	// earlier update's context with a later update's discovery state.
	ctx := s.Context()
	if err := ctx.Err(); err != nil {
		s.discoveryMu.Unlock()
		return nil, err
	}
	round := s.discovery
	if round == nil {
		s.discoveryMu.Unlock()
		out, err := s.fetchInstruments(ctx)
		return append([]protocol.ExInstrument(nil), out...), err
	}
	if round.items != nil {
		out := append([]protocol.ExInstrument(nil), round.items...)
		s.discoveryMu.Unlock()
		return out, nil
	}
	if pending := round.pending; pending != nil {
		s.discoveryMu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pending.done:
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return append([]protocol.ExInstrument(nil), pending.items...), pending.err
		}
	}
	pending := &instrumentRequest{done: make(chan struct{})}
	round.pending = pending
	s.discoveryMu.Unlock()

	// No state mutex is held while borrowing a connection or fetching pages.
	out, err := s.fetchInstruments(ctx)
	s.discoveryMu.Lock()
	if err == nil {
		round.items = out
	}
	round.pending = nil // Failed requests may be retried by a subsequent caller.
	pending.items, pending.err = out, err
	close(pending.done)
	s.discoveryMu.Unlock()
	return append([]protocol.ExInstrument(nil), out...), err
}

func (s *Service) fetchInstruments(ctx context.Context) ([]protocol.ExInstrument, error) {
	p := s.ExPool()
	if p == nil {
		return nil, fmt.Errorf("pull: 需要配置 ExPool（扩展行情7727连接池）")
	}
	var out []protocol.ExInstrument
	err := p.DoContext(ctx, func(c tdx.Client) error {
		var err error
		out, err = tdx.WithContext(ctx, c).GetInstruments()
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []protocol.ExInstrument{}
	}
	return out, nil
}
