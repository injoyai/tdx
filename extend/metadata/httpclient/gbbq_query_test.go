package httpclient

import (
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/protocol"
)

type recordClient struct {
	tdx.Client
	records map[string][]*protocol.Gbbq
}

func (c *recordClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) { return c.records, nil }

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
	remote := &GbbqHTTP{snapshot: local}
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
	if !reflect.DeepEqual(local.GetXRXDs("000001"), remote.GetXRXDs("sz000001")) || len(local.GetXRXDs("000001")) != 1 {
		t.Fatal("XRXD queries differ")
	}
	if !reflect.DeepEqual(local.GetFactors("000001", ks), remote.GetFactors("000001", ks)) {
		t.Fatal("factor queries differ")
	}
	for _, source := range []gbbqQuery{local, remote} {
		rate, available := source.Turnover("600000", day, 1)
		if source.GetEquity("600000", day) != nil || available || rate != 0 || source.GetXRXDs("600000") == nil || len(source.GetXRXDs("600000")) != 0 {
			t.Fatal("empty result contract changed")
		}
	}
}
