package export

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestCSVContracts(t *testing.T) {
	at := time.Date(2026, 1, 5, 9, 31, 0, 0, protocol.Location())
	for _, tc := range []struct {
		name  string
		write func(string) error
		want  [][]string
	}{
		{"kline", func(path string) error {
			return KlinesToCSV(path, "sz000001", "名称,含\"引号\"", protocol.Klines{{Time: at, Open: 10000, High: 11000, Low: 9000, Close: 10500, Volume: 2, Amount: 2100000}})
		}, [][]string{
			{"日期", "时间", "代码", "名称", "开盘(元)", "最高(元)", "最低(元)", "收盘(元)", "成交量(股)", "成交额(元)"},
			{"2026-01-05", "09:31", "sz000001", "名称,含\"引号\"", "10", "11", "9", "10.5", "200", "2100"},
		}},
		{"trade", func(path string) error {
			return TradesToCSV(path, protocol.Trades{{Time: at, Price: 10500, Volume: 2, Status: 1}})
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
