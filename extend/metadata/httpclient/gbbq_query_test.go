package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/protocol"
)

func fetchTestGbbq(t *testing.T, records []*protocol.Gbbq) *metadata.Gbbq {
	t.Helper()
	groups := map[string][]*protocol.Gbbq{}
	for _, row := range records {
		groups[row.Code] = append(groups[row.Code], row)
	}
	body, err := json.Marshal(groups)
	if err != nil {
		t.Fatal(err)
	}
	c := New("http://offline.invalid", &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})})
	snapshot, err := c.FetchGbbq(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

type gbbqQuery interface {
	GetEquity(string, time.Time) *protocol.Equity
	Turnover(string, time.Time, int64) (float64, bool)
	GetXRXDs(string) protocol.XRXDs
	GetFactors(string, protocol.Klines) []*protocol.Factor
}

func TestGbbqQueryEffectiveDate(t *testing.T) {
	loc := time.FixedZone("sample", 8*3600)
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, loc)
	old := &protocol.Gbbq{Code: "sz000001", Time: day.AddDate(0, 0, -1).Add(15 * time.Hour), Category: 5, C3: 100, C4: 200}
	event := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 1, C1: 1}
	current := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 5, C3: 300, C4: 400}
	records := map[string][]*protocol.Gbbq{"sz000001": {old, event, current}}
	var flat []*protocol.Gbbq
	for _, rows := range records {
		flat = append(flat, rows...)
	}
	local, err := metadata.NewGbbq(flat)
	if err != nil {
		t.Fatal(err)
	}
	remote := fetchTestGbbq(t, flat)
	for _, tc := range []struct {
		at   time.Time
		want int64
	}{
		{day.AddDate(0, 0, -2), 0}, {day.Add(-time.Minute), 100}, {day, 300}, {day.Add(9 * time.Hour), 300}, {day.AddDate(0, 0, 1), 300},
	} {
		for _, source := range []gbbqQuery{local, remote} {
			eq := source.GetEquity("000001", tc.at)
			rate, available := source.Turnover("000001", tc.at, 3)
			if tc.want == 0 {
				if eq != nil || available || rate != 0 {
					t.Fatal("unexpected equity", eq)
				}
				continue
			}
			if eq == nil || eq.Float != tc.want || !available || rate != 300/float64(tc.want) {
				t.Fatal(eq, tc.want)
			}
		}
	}
	ks := protocol.Klines{{Time: day.AddDate(0, 0, -1), Open: 10, High: 10, Low: 10, Close: 10}, {Time: day, Open: 9, High: 9, Low: 9, Close: 9}}
	// HTTP 保留时间及偏移，不保留 time.Location 的内存身份和名字。
	sameJSON := func(a, b any) bool {
		x, err := json.Marshal(a)
		if err != nil {
			t.Fatal(err)
		}
		y, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Equal(x, y)
	}
	if !sameJSON(local.GetXRXDs("000001"), remote.GetXRXDs("sz000001")) || len(local.GetXRXDs("000001")) != 1 {
		t.Fatal("XRXD queries differ")
	}
	if !sameJSON(local.GetFactors("000001", ks), remote.GetFactors("000001", ks)) {
		t.Fatal("factor queries differ")
	}
	for _, source := range []gbbqQuery{local, remote} {
		rate, available := source.Turnover("600000", day, 1)
		if source.GetEquity("600000", day) != nil || available || rate != 0 || source.GetXRXDs("600000") == nil || len(source.GetXRXDs("600000")) != 0 {
			t.Fatal("empty result contract changed")
		}
	}
}
