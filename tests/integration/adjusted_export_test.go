package integration_test

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/export"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type adjustedIndexClient struct {
	tdx.Client
	raw *protocol.Kline
}

func (*adjustedIndexClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{}, nil
}
func (c *adjustedIndexClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	k := c.raw
	return &tdx.KlineResp{Instrument: protocol.Instrument{Exchange: protocol.ExchangeSH, Code: "000001"}, List: []tdx.Kline{{Time: k.Time.Format("2006-01-02"), Volume: k.Volume, Standard: k}}}, nil
}

func TestAdjustedIndexExportsAndIndicatorsUseLots(t *testing.T) {
	raw := &protocol.Kline{Time: consistencyDay(19), Open: 10.8, High: 10.8, Low: 10.8, Close: 10.8, Volume: 20000, Amount: 216000}
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	g, err := gbbq.New(gbbq.WithDB(db), gbbq.WithClient(&adjustedIndexClient{raw: raw}), gbbq.WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	for _, fetch := range []func(string) (protocol.Klines, error){g.QFQKlineDay, g.HFQKlineDay} {
		ks, err := fetch("sh000001")
		if err != nil || len(ks) != 1 {
			t.Fatal(ks, err)
		}
		if ks[0].Volume != 200 || raw.Volume != 20000 || ks.VWAP() != 10.8 {
			t.Fatal("index units changed", ks[0], raw, ks.VWAP())
		}
		path := filepath.Join(t.TempDir(), "index.csv")
		if err := export.KlinesToCSV(path, "sh000001", "index", ks); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{239, 187, 191}))).ReadAll()
		if err != nil || len(rows) != 2 || rows[1][8] != "20000" || rows[1][9] != "216000" {
			t.Fatal("exported wrong volume/amount", rows, err)
		}
	}
}
