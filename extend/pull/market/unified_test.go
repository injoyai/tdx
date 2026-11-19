package market

import (
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

type unifiedClient struct {
	tdx.Client
	count  int
	amount *float64
	code   string
	day    string
}

func (*unifiedClient) Close() error { return nil }

func (*unifiedClient) GetInstruments() ([]protocol.ExInstrument, error) {
	return []protocol.ExInstrument{{Market: 31, Code: "00700"}, {Market: 49, Code: "02800"}, {Market: 49, Code: "00823"}}, nil
}

func (c *unifiedClient) GetKlineUntil(category uint8, code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	c.code = code
	at := "2026-01-05"
	if c.day != "" {
		at = c.day
	}
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
	u := &stdUnit{name: protocol.MarketSZ}
	rows, err := u.minUntil(c, "sz000001", "2026-01-01 00:00")
	if err != nil || len(rows) != c.count || rows[0].Volume != 12345 {
		t.Fatalf("unified history truncated or rescaled: count=%d err=%v", len(rows), err)
	}
}

func TestUnitsStoreUnifiedKlines(t *testing.T) {
	for _, tc := range []struct{ input, request string }{
		{"sz000001", "sz000001"}, {"sh000001", "sh000001"},
		{"sh510300", "sh510300"}, {"sh880001", "sh880001"},
		{"sh110075", "sh110075"}, {"sh019001", "sh019001"},
		{"hk00700", "00700.31"}, {"hk.HSI", "hk.HSI"}, {"usAAPL", "us.AAPL"},
		{"hk02800", "02800.49"}, {"hk.00823", "00823.49"}, {"hk/fund.00823", "00823.49"},
		{"cffIF2609", "cff.IF2609"}, {"czcTA2609", "czc.TA2609"},
		{"dceM2609", "dce.M2609"}, {"shfcu2609", "shf.cu2609"},
		{"gfeSI2609", "gfe.SI2609"}, {"qhzIFL9", "qhz.IFL9"}, {"qhz.IMCI", "qhz.IMCI"},
		{"wi.A_IXIC", "wi.A_IXIC"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			amount := 9876.54321
			c := &unifiedClient{amount: &amount}
			pool, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			cfg := &pull.Config{Store: testStore(t.TempDir()), StartAt: "20260101", Codes: []string{tc.input},
				Pool: pool, ExPool: pool}
			if tc.input == "wi.A_IXIC" {
				if s, err := pull.New(cfg); err == nil || !strings.Contains(err.Error(), "显式配置 Schedule") {
					if s != nil {
						_ = s.Close()
					}
					t.Fatalf("missing schedule: %v", err)
				}
				cfg.Schedules = map[protocol.MarketID]pull.Schedule{
					protocol.MarketInternationalIndex: {TimeZone: "America/New_York", CloseTime: "16:00", Delay: 5 * time.Minute},
				}
			}
			s, err := pull.New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			code, err := s.ParseCode(tc.input)
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
			if c.code != tc.request {
				t.Fatalf("day request code = %q, want %q", c.code, tc.request)
			}
			days, err := s.QueryDay(code, "", "")
			if err != nil || len(days) != 1 {
				t.Fatal(days, err)
			}
			if k := days[0]; k.Time != "2026-01-05" || k.Open != 10.12345 || k.Low != -1.25 || k.Close != 11.23456 || k.Volume != 12345 || (k.Amount == nil || *k.Amount != amount) {
				t.Fatalf("normalized day changed: %+v", k)
			}
			// 无金额时保留未知状态，不能把扩展记录的持仓字节当作金额。
			c.amount = nil
			if err := u.FetchMinute(s, code); err != nil {
				t.Fatal(err)
			}
			if c.code != tc.request {
				t.Fatalf("minute request code = %q, want %q", c.code, tc.request)
			}
			rows, err := s.QueryMinute(code, "", "")
			if err != nil || len(rows) != 1 {
				t.Fatal(rows, err)
			}
			if k := rows[0]; k.Time != "2026-01-05 09:31" || k.Open != 10.12345 || k.Low != -1.25 || k.Close != 11.23456 || k.Volume != 12345 || k.Amount != nil {
				t.Fatalf("normalized minute changed: %+v", k)
			}
		})
	}
}

func TestExtendedMarketPrefixValidation(t *testing.T) {
	for _, code := range []string{"unknown/IF2609", "47/IF2609", "CFF/IF2609", "us/AAPL", "czc/TA2609"} {
		if _, err := (model.Code{Market: protocol.MarketCFF, Code: code}).Instrument(); err == nil {
			t.Errorf("accepted noncanonical storage code: %q", code)
		}
	}
	if _, err := (model.Code{Code: "AAPL"}).Instrument(); err == nil {
		t.Fatal("accepted a code without a market")
	}
}
