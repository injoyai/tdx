package metadatahttp

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestGbbqHTTPEffectiveDay(t *testing.T) {
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	old := &protocol.Gbbq{Code: "sz000001", Time: day.Add(-9 * time.Hour), Category: 5, C3: 100, C4: 200}
	current := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 5, C3: 300, C4: 400}
	g := &GbbqHTTP{m: map[string][]*protocol.Gbbq{"sz000001": {old, current}}}
	for _, tc := range []struct {
		at   time.Time
		want int64
	}{
		{day.Add(-time.Minute), 100}, {day, 300}, {day.Add(9*time.Hour + 30*time.Minute), 300}, {current.Time, 300},
	} {
		if got := g.GetEquity("sz000001", tc.at); got == nil || got.Float != tc.want {
			t.Fatalf("equity at %s = %+v, want %d", tc.at, got, tc.want)
		}
	}
}

func TestGbbqHTTPCodeForms(t *testing.T) {
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, protocol.Location())
	g := &GbbqHTTP{m: map[string][]*protocol.Gbbq{"sz000001": {{Time: at, Category: 5, C3: 100, C4: 200}}}}
	for _, code := range []string{"sz000001", "SZ000001", "sz.000001", "000001.SZ", "000001"} {
		if got := g.GetEquity(code, at); got == nil || got.Float != 100 {
			t.Fatalf("%s: %+v", code, got)
		}
	}
	for _, code := range []string{"hk00001", "invalid", ""} {
		if got := g.GetEquity(code, at); got != nil {
			t.Fatalf("invalid code %s: %+v", code, got)
		}
	}
}
