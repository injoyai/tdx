package integration_test

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/export"
	"github.com/injoyai/tdx/v2/extend/metadata"
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
	g, err := metadata.NewGbbq(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, adjust := range []func(string, protocol.Klines) protocol.Klines{g.QFQ, g.HFQ} {
		response, err := (&adjustedIndexClient{raw: raw}).GetKlineDayAll("sh000001")
		if err != nil {
			t.Fatal(err)
		}
		ks, err := response.Klines()
		if err != nil {
			t.Fatal(err)
		}
		ks = adjust("sh000001", ks)
		if ks[0].Volume != 200 || raw.Volume != 20000 || ks.VWAP() != 10.8 {
			t.Fatal("index units changed", ks[0], raw, ks.VWAP())
		}
		path := filepath.Join(t.TempDir(), "index.csv")
		rowsToExport := make([]tdx.Kline, 0, len(ks))
		for _, k := range ks {
			amount := k.Amount
			rowsToExport = append(rowsToExport, tdx.Kline{Time: k.Time.Format(protocol.TimeLayout), Open: k.Open,
				High: k.High, Low: k.Low, Close: k.Close, Volume: protocol.ToShares(k.Volume), Amount: &amount})
		}
		if err := export.KlinesToCSV(path, "sh000001", "index", rowsToExport); err != nil {
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
