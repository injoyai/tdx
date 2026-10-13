package extend

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCodesHTTPStartupErrors(t *testing.T) {
	want := errors.New("offline initialization failure")
	options := []tdx.CodesOption{tdx.WithCodesDialClient(func() (tdx.Client, error) { return nil, want })}
	for _, test := range []struct {
		name string
		fn   func() error
	}{
		{"codes", func() error { return ListenCodesHTTP(0, options...) }},
		{"codes and equity", func() error { return ListenCodesAndGbbqHTTP(0, options, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); !errors.Is(err, want) {
				t.Errorf("initialization failure reported as success: got=%v", err)
			}
		})
	}
}

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
