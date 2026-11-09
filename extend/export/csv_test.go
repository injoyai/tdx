package export

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCSVContracts(t *testing.T) {
	at := time.Date(2026, 1, 5, 9, 31, 0, 0, protocol.Location())
	amount, price := 2100.0, 10.5
	for _, tc := range []struct {
		name  string
		write func(string) error
		want  [][]string
	}{
		{"kline", func(path string) error {
			return KlinesToCSV(path, "sz000001", "名称,含\"引号\"", []tdx.Kline{{Time: at.Format(protocol.TimeLayout), Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 200, Amount: &amount}})
		}, [][]string{
			{"日期", "时间", "代码", "名称", "开盘(元)", "最高(元)", "最低(元)", "收盘(元)", "成交量(股)", "成交额(元)"},
			{"2026-01-05", "09:31", "sz000001", "名称,含\"引号\"", "10", "11", "9", "10.5", "200", "2100"},
		}},
		{"trade", func(path string) error {
			return TradesToCSV(path, []tdx.Trade{{Date: "20260105", Time: "09:31:00", Price: &price, Volume: 200, Direction: -1}})
		}, [][]string{
			{"日期", "时间", "价格(元)", "成交量(股)", "成交额(元)", "方向(0买,1卖,2中性)"},
			{"2026-01-05", "09:31", "10.5", "200", "2100", "1"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "nested", tc.name+".csv")
			if err := tc.write(path); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bom := []byte{0xef, 0xbb, 0xbf}
			if !bytes.HasPrefix(data, bom) {
				t.Fatal("missing UTF-8 BOM")
			}
			rows, err := csv.NewReader(bytes.NewReader(data[len(bom):])).ReadAll()
			if err != nil || !reflect.DeepEqual(rows, tc.want) {
				t.Fatalf("rows=%v, want %v; err=%v", rows, tc.want, err)
			}
			if err := tc.write(filepath.Join(path, "blocked.csv")); err == nil {
				t.Fatal("writing through a regular file must fail")
			}
		})
	}
}

func TestCSVPreservesUnifiedSharesAndUnknownValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "day.csv")
	if err := KlinesToCSV(path, "sz000001", "", []tdx.Kline{{Time: "2026-01-05", Volume: 123}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil || rows[1][1] != "" || rows[1][8] != "123" || rows[1][9] != "" {
		t.Fatal(rows, err)
	}
	if err := TradesToCSV(path, []tdx.Trade{{Date: "20260105", Time: "09:31:00", Volume: 123}}); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(bytes.NewReader(data[3:])).ReadAll()
	if err != nil || rows[1][2] != "" || rows[1][3] != "123" || rows[1][4] != "" || rows[1][5] != "2" {
		t.Fatal(rows, err)
	}
	before := append([]byte(nil), data...)
	for _, write := range []func() error{
		func() error { return KlinesToCSV(path, "sz000001", "", []tdx.Kline{{Time: "bad"}}) },
		func() error { return TradesToCSV(path, []tdx.Trade{{Date: "20260230", Time: "09:31:00"}}) },
		func() error { return TradesToCSV(path, []tdx.Trade{{Time: "24:01:00"}}) },
		func() error { return TradesToCSV(path, []tdx.Trade{{Time: "09:31:00", Direction: 9}}) },
	} {
		if err := write(); err == nil {
			t.Fatal("invalid CSV input accepted")
		}
	}
	data, err = os.ReadFile(path)
	if err != nil || !bytes.Equal(data, before) {
		t.Fatal("invalid input overwrote existing file", err)
	}
}
