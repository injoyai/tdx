package acceptance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

type snapshotPool struct{ tdx.Client }

func (p snapshotPool) DoContext(ctx context.Context, fn func(tdx.Client) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(p.Client)
}

type snapshotSource struct{ tdx.Client }

func (snapshotSource) GetCodeAll(ex protocol.Exchange) (*tdx.CodeResp, error) {
	if ex != 238 && ex != 239 {
		return &tdx.CodeResp{}, nil
	}
	return &tdx.CodeResp{Count: 1, List: []tdx.Code{
		{Standard: &protocol.Code{Code: "600000"}},
	}}, nil
}

type snapshotMarket struct{ info protocol.MarketInfo }

func (m snapshotMarket) Info() protocol.MarketInfo { return m.info }

func (snapshotMarket) Resolve(c protocol.MarketCode, _ protocol.InstrumentSnapshot) (protocol.Instrument, error) {
	if len(c.Symbol) != 6 {
		return protocol.Instrument{}, errors.New("invalid standard symbol")
	}
	return protocol.Instrument{Exchange: c.Exchange, Code: c.Symbol}, nil
}

func (snapshotMarket) Rules(protocol.Instrument) (protocol.InstrumentRules, error) {
	return protocol.StandardRules(protocol.SecurityStock), nil
}

func TestFetchCodesKeepsStandardRoutes(t *testing.T) {
	const one, two protocol.MarketID = "stdmeta", "stdmeta/other"
	const exOne, exTwo protocol.Exchange = 239, 238
	if _, known := one.Exchange(); !known {
		m := snapshotMarket{protocol.MarketInfo{Prefix: "stdmeta", Routes: []protocol.RouteInfo{
			{ID: one, Exchange: exOne, Protocol: protocol.ProtocolStandard},
			{ID: two, Exchange: exTwo, Protocol: protocol.ProtocolStandard},
		}}}
		if err := protocol.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	s, err := pull.New(&pull.Config{Pool: snapshotPool{snapshotSource{}}, Retry: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	all, err := s.FetchCodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Both routes advertise the same bare code. The first route shares its key
	// with the logical prefix, so its canonical query uses the exact wire number.
	cache, err := metadata.NewCodes(all.Rows)
	if err != nil {
		t.Fatal(err)
	}
	firstCode, secondCode := "600000.239", "stdmeta/other.600000"
	first, second := cache.Get(firstCode), cache.Get(secondCode)
	if first == nil || second == nil || first.Exchange != one.String() || second.Exchange != two.String() ||
		first.FullCode() != firstCode || second.FullCode() != secondCode {
		t.Fatal(first, second)
	}

	backend, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.ReplaceCodes(ctx, all); err != nil {
		t.Fatal(err)
	}
	empty := model.CodeSnapshot{Exchanges: []protocol.Exchange{exTwo}, FetchedAt: all.FetchedAt.Add(time.Minute)}
	if err := backend.ReplaceCodes(ctx, empty); err != nil {
		t.Fatal(err)
	}
	rest, err := backend.LoadCodes(ctx, []protocol.Exchange{exOne, exTwo})
	if err != nil || len(rest.Rows) != 1 || rest.Rows[0].FullCode() != firstCode || !rest.FetchedAt.Equal(all.FetchedAt) {
		t.Fatal(rest, err)
	}
	filtered, err := backend.LoadCodes(ctx, []protocol.Exchange{exTwo})
	if err != nil || len(filtered.Rows) != 0 || !filtered.FetchedAt.Equal(empty.FetchedAt) {
		t.Fatal(filtered, err)
	}

	all.Exchanges = []protocol.Exchange{exOne}
	if err := all.Validate(); err == nil {
		t.Fatal("snapshot accepted rows from outside its route scope")
	}
}
