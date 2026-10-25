package market

import (
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

type unifiedClient struct {
	tdx.Client
	count  int
	amount *float64
}

func (*unifiedClient) Close() error { return nil }

func (c *unifiedClient) GetKlineUntil(category uint8, _ string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	at := "2026-01-05"
	if protocol.IsIntraday(category) {
		at += " 09:31"
	}
	// 原始字段刻意不同，拉取层必须使用 Client 已归一化的字段。
	row := tdx.Kline{Time: at, Open: 10.12345, High: 12.5, Low: -1.25, Close: 11.23456,
		Volume: 12345, Amount: c.amount, Standard: &protocol.Kline{Volume: 1},
		Extended: &protocol.ExKline{Trade: 1, Position: 100000}}
	_ = stop(row)
	rows := make([]tdx.Kline, max(c.count, 1))
	for i := range rows {
		rows[i] = row
	}
	return &tdx.KlineResp{Count: len(rows), List: rows}, nil
}

func (c *unifiedClient) GetKlineMinute241Until(code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineMinute, code, stop)
}

func TestStandardMinuteBeyondUint16(t *testing.T) {
	c := &unifiedClient{count: 65536}
	u := &stdUnit{name: protocol.MarketAStock}
	rows, err := u.minUntil(c, "sz000001", "2026-01-01 00:00")
	if err != nil || len(rows) != c.count || rows[0].Volume != 12345 {
		t.Fatalf("unified history truncated or rescaled: count=%d err=%v", len(rows), err)
	}
}

func TestUnitsStoreUnifiedKlines(t *testing.T) {
	for _, input := range []string{"sz000001", "sh000001", "sh510300", "sh880001", "sh110075", "sh019001", "hk00700", "usAAPL", "cffIF2609"} {
		t.Run(input, func(t *testing.T) {
			amount := 9876.54321
			c := &unifiedClient{amount: &amount}
			pool, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20260101", Codes: []string{input},
				Manage: &manage.Manage{ClientPool: pool}, ExPool: pool})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			code, err := pull.ParseCode(input)
			if err != nil {
				t.Fatal(err)
			}
			u, ok := pull.Get(code.Market.String())
			if !ok {
				t.Fatal("missing unit", code.Market)
			}
			if err := u.FetchDay(s, code); err != nil {
				t.Fatal(err)
			}
			days, err := s.QueryDay(code, "", "")
			if err != nil || len(days) != 1 {
				t.Fatal(days, err)
			}
			if k := days[0]; k.Time != "2026-01-05" || k.Open != 10.12345 || k.Low != -1.25 || k.Close != 11.23456 || k.Volume != 12345 || k.Amount != amount {
				t.Fatalf("normalized day changed: %+v", k)
			}
			// 无金额时保存零值，不能把扩展记录的持仓字节当作金额。
			c.amount = nil
			if err := u.FetchMinute(s, code); err != nil {
				t.Fatal(err)
			}
			rows, err := s.QueryMinute(code, "", "")
			if err != nil || len(rows) != 1 {
				t.Fatal(rows, err)
			}
			if k := rows[0]; k.Time != "2026-01-05 09:31" || k.Open != 10.12345 || k.Low != -1.25 || k.Close != 11.23456 || k.Volume != 12345 || k.Amount != 0 {
				t.Fatalf("normalized minute changed: %+v", k)
			}
		})
	}
}
