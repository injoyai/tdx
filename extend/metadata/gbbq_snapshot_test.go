package metadata

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// TestQFQUsesTradingDay 回归 F01 的公开复权入口: 事件在 15:00, 本地日线在 00:00,
// 同一交易日必须按同一阶段处理。前日收盘 10、除权日收盘 9, 每 10 股派 10 元。
func TestQFQUsesTradingDay(t *testing.T) {
	event := time.Date(2026, 11, 20, 15, 0, 0, 0, protocol.Location())
	g, err := NewGbbq([]*model.Gbbq{{Code: "sz000001", Time: event, Category: 1, C1: 10}})
	if err != nil {
		t.Fatal(err)
	}
	for _, hour := range []int{0, 15} {
		ks := protocol.Klines{
			dayKline(20261119, hour, 10),
			dayKline(20261120, hour, 9),
		}
		out := g.QFQ("sz000001", ks)
		if out[0].Close != 9 || out[1].Close != 9 {
			t.Fatalf("hour=%d 前复权收盘 = %v, %v; want 9, 9", hour, out[0].Close, out[1].Close)
		}
	}
}

func dayKline(yyyymmdd, hour int, close float64) *protocol.Kline {
	t := time.Date(yyyymmdd/10000, time.Month(yyyymmdd/100%100), yyyymmdd%100, hour, 0, 0, 0, protocol.Location())
	return &protocol.Kline{Time: t, Open: close, High: close, Low: close, Close: close, Last: close}
}

func TestHistoricalEquityAndSnapshotOwnership(t *testing.T) {
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	old := &model.Gbbq{Code: "sz000001", Time: day.AddDate(0, 0, -1), Category: 5, C3: 100, C4: 200}
	newer := &model.Gbbq{Code: "sz000001", Time: day, Category: 5, C3: 300, C4: 400}
	g, err := NewGbbq([]*model.Gbbq{newer, old})
	if err != nil {
		t.Fatal(err)
	}
	old.C3 = 999
	g.All()["sz000001"][0].C3 = 888
	for _, tc := range []struct {
		at   time.Time
		want int64
	}{
		{day.AddDate(0, 0, -2), 0}, {day.AddDate(0, 0, -1), 100}, {day.Add(-15 * time.Hour), 300},
	} {
		e := g.GetEquity("000001", tc.at)
		if tc.want == 0 {
			if e != nil {
				t.Fatal(e)
			}
			continue
		}
		if e == nil || e.Float != tc.want {
			t.Fatal(tc, e)
		}
	}
	if v, ok := g.Turnover("sz000001", day.AddDate(0, 0, -2), 0); ok || v != 0 {
		t.Fatal("missing equity reported as available")
	}
	if v, ok := g.Turnover("sz000001", day, 0); !ok || v != 0 {
		t.Fatal("actual zero turnover lost")
	}
	for _, bad := range [][]*model.Gbbq{{nil}, {{Code: "invalid", Time: day}}, {{Code: "sz000001"}}} {
		if _, err := NewGbbq(bad); err == nil {
			t.Fatal("accepted invalid event", bad)
		}
	}
}
