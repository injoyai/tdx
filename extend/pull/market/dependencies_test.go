package market

import (
	"context"
	"strings"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

type metadataPool struct {
	tdx.ClientPool
	c      tdx.Client
	closed bool
}

func (p *metadataPool) DoContext(ctx context.Context, f func(tdx.Client) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f(p.c)
}
func (p *metadataPool) Close() error { p.closed = true; return nil }

type metadataCodes struct {
	*metadata.Codes
	closed bool
}

func (c *metadataCodes) Close() error { c.closed = true; return nil }

type metadataClient struct{ tdx.Client }

func (*metadataClient) GetKlineUntil(uint8, string, func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	return &tdx.KlineResp{List: []tdx.Kline{
		{Time: "2026-01-05", Close: 10, Volume: 100},
		{Time: "2026-01-06", Close: 11, Volume: 100},
	}}, nil
}

func TestDirectMetadataDependencies(t *testing.T) {
	pool := &metadataPool{c: &metadataClient{}}
	index, err := metadata.NewCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	cache := &metadataCodes{Codes: index}
	if err := cache.Update([]*model.CodeModel{
		{Exchange: "sz", Code: "000001", Name: "股票"},
		{Exchange: "sh", Code: "000001", Name: "指数"},
		{Exchange: "sh", Code: "510300", Name: "基金"},
		{Exchange: "sh", Code: "110075", Name: "可转债"},
		{Exchange: "sh", Code: "019001", Name: "其他债券"},
	}); err != nil {
		t.Error(err)
	}
	s, err := pull.New(&pull.Config{Store: testStore(t.TempDir()), StartAt: "20260101",
		Pool: pool, ListCodes: func(context.Context) ([]model.Code, error) {
			list := []model.Code{}
			for _, row := range cache.Iter() {
				i, err := protocol.ParseInstrument(row.FullCode())
				if err != nil {
					return nil, err
				}
				market, err := i.Market()
				if err != nil {
					return nil, err
				}
				list = append(list, model.Code{Market: market, Code: row.FullCode(), Name: row.Name})
			}
			return list, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, tc := range []struct {
		market protocol.Market
		code   string
	}{
		{protocol.MarketAStock, "sz000001"},
		{protocol.MarketIndex, "sh000001"},
		{protocol.MarketEtfLof, "sh510300"},
		{protocol.MarketBond, "sh110075"},
		{protocol.MarketOther, "sh019001"},
	} {
		u, _ := pull.Get(tc.market.String())
		list, err := u.Codes(s)
		if err != nil || len(list) != 1 || list[0].Code != tc.code || list[0].Name == "" {
			t.Fatalf("discovery %s: codes=%v err=%v", tc.market, list, err)
		}
		if err := u.FetchDay(s, list[0]); err != nil {
			t.Fatal(err)
		}
		rows, err := s.QueryDay(list[0], "", "")
		if err != nil || len(rows) != 2 {
			t.Fatal(rows, err)
		}
	}

	// 下一轮读取同一个缓存的新快照，不复制成永久不变的代码表。
	if err := cache.Update([]*model.CodeModel{{Exchange: "sh", Code: "600000", Name: "新股票"}}); err != nil {
		t.Error(err)
	}
	u, _ := pull.Get(protocol.MarketAStock.String())
	list, err := u.Codes(s)
	if err != nil || len(list) != 1 || list[0].Code != "sh600000" {
		t.Fatal("did not reuse refreshed metadata", list, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if pool.closed || cache.closed {
		t.Fatal("pull closed caller-owned dependencies")
	}
}

func TestMissingStandardDependencies(t *testing.T) {
	s, err := pull.New(&pull.Config{Store: testStore(t.TempDir()), Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, market := range []protocol.Market{protocol.MarketAStock, protocol.MarketIndex,
		protocol.MarketEtfLof, protocol.MarketBond, protocol.MarketOther} {
		u, _ := pull.Get(market.String())
		if _, err := u.Codes(s); err == nil || !strings.Contains(err.Error(), "ListCodes") {
			t.Fatalf("missing cache %s: %v", market, err)
		}
	}
	u, _ := pull.Get(protocol.MarketAStock.String())
	code := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	if err := u.FetchDay(s, code); err == nil || !strings.Contains(err.Error(), "Pool") {
		t.Fatal("missing pool", err)
	}
}

func TestSpecifiedMinuteNeedsOnlyPool(t *testing.T) {
	pool := &metadataPool{c: &unifiedClient{}}
	s, err := pull.New(&pull.Config{Store: testStore(t.TempDir()), StartAt: "20260101",
		Codes: []string{"sz000001"}, Minute: true, Pool: pool})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	code, err := pull.ParseCode("sz000001")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 1 || rows[0].Volume != 12345 {
		t.Fatal("specified minute pull without metadata failed", rows, err)
	}
}
