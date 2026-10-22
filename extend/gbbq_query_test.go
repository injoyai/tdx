package extend

import (
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type recordClient struct {
	tdx.Client
	records map[string][]*protocol.Gbbq
}

func (c *recordClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) { return c.records, nil }

func TestGbbqQueryEffectiveDate(t *testing.T) {
	loc := time.FixedZone("sample", 8*3600)
	day := time.Date(2026, 1, 5, 0, 0, 0, 0, loc)
	old := &protocol.Gbbq{Code: "sz000001", Time: day.AddDate(0, 0, -1).Add(15 * time.Hour), Category: 5, C3: 100, C4: 200}
	event := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 1, C1: 1}
	current := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 5, C3: 300, C4: 400}
	records := map[string][]*protocol.Gbbq{"sz000001": {old, event, current}}
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	local, err := gbbq.New(gbbq.WithDB(db), gbbq.WithClient(&recordClient{records: records}), gbbq.WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	remote := &GbbqHTTP{m: records}
	for _, tc := range []struct {
		at   time.Time
		want int64
	}{
		{day.AddDate(0, 0, -2), 0}, {day.Add(-time.Minute), 100}, {day, 300}, {day.Add(9 * time.Hour), 300}, {day.AddDate(0, 0, 1), 300},
	} {
		for _, source := range []gbbq.Interface{local, remote} {
			eq := source.GetEquity("000001", tc.at)
			if tc.want == 0 {
				if eq != nil || source.GetTurnover("000001", tc.at, 3) != 0 {
					t.Fatal("unexpected equity", eq)
				}
				continue
			}
			if eq == nil || eq.Float != tc.want || source.GetTurnover("000001", tc.at, 3) != 300/float64(tc.want) {
				t.Fatal(eq, tc.want)
			}
		}
	}
	ks := protocol.Klines{{Time: day.AddDate(0, 0, -1), Open: 10000, High: 10000, Low: 10000, Close: 10000}, {Time: day, Open: 9000, High: 9000, Low: 9000, Close: 9000}}
	if !reflect.DeepEqual(local.GetXRXDs("000001"), remote.GetXRXDs("sz000001")) || len(local.GetXRXDs("000001")) != 1 {
		t.Fatal("XRXD queries differ")
	}
	if !reflect.DeepEqual(local.GetFactors("000001", ks), remote.GetFactors("000001", ks)) {
		t.Fatal("factor queries differ")
	}
	for _, source := range []gbbq.Interface{local, remote} {
		if source.GetEquity("600000", day) != nil || source.GetTurnover("600000", day, 1) != 0 || source.GetXRXDs("600000") == nil || len(source.GetXRXDs("600000")) != 0 {
			t.Fatal("empty result contract changed")
		}
	}
}
