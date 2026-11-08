package httpclient

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestGbbqHTTPValidatedSnapshot(t *testing.T) {
	day := time.Date(2026, 1, 6, 0, 0, 0, 0, protocol.Location())
	newer := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 5, C3: 300, C4: 400}
	older := &protocol.Gbbq{Code: "sz000001", Time: day.AddDate(0, 0, -1), Category: 5, C3: 100, C4: 200}
	body, err := json.Marshal(map[string][]*protocol.Gbbq{"SZ000001": {newer, older}})
	if err != nil {
		t.Fatal(err)
	}
	payload := string(body)
	previous := dataHTTPClient
	t.Cleanup(func() { dataHTTPClient = previous })
	dataHTTPClient = &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	g := &GbbqHTTP{address: "http://offline.invalid", httpUpdates: newHTTPUpdates()}
	defer g.Close()
	if err := g.Update(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   time.Time
		want int64
	}{{day.Add(-time.Minute), 100}, {day, 300}} {
		if eq := g.GetEquity("000001", tc.at); eq == nil || eq.Float != tc.want {
			t.Fatalf("equity at %s = %+v, want %d", tc.at, eq, tc.want)
		}
	}
	for _, tc := range []struct{ name, body string }{
		{"null event", `{"sz000001":[null]}`},
		{"missing date", `{"sz000001":[{"Code":"sz000001","Category":5,"C3":999}]}`},
		{"invalid code", `{"sz000001":[{"Code":"bad","Time":"2026-01-06T15:00:00+08:00"}]}`},
		{"wrong security", `{"sh600000":[{"Code":"sz000001","Time":"2026-01-06T15:00:00+08:00"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload = tc.body
			if err := g.Update(); err == nil {
				t.Fatal("invalid response accepted")
			}
			if eq := g.GetEquity("sz000001", day); eq == nil || eq.Float != 300 {
				t.Fatalf("failed update replaced snapshot: %+v", eq)
			}
		})
	}
	payload = string(body)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if eq := g.GetEquity("sz000001", day); eq == nil || eq.Float != 300 {
					t.Errorf("concurrent query: %+v", eq)
				}
				g.Turnover("sz000001", day, 3)
				g.GetXRXDs("sz000001")
				g.GetFactors("sz000001", nil)
			}
		}()
	}
	for i := 0; i < 20; i++ {
		if err := g.Update(); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}
