package metadata

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

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
