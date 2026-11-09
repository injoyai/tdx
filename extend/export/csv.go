// Package export 将统一 Client 的标准行情数据导出为带 UTF-8 BOM 的 CSV 文件。
package export

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// KlinesToCSV 接受统一 Client 的 K 线，成交量直接按股输出；未知成交额留空。
// 日线时间列为空；本入口表头适用于人民币、股数口径的数据。
func KlinesToCSV(filename string, code, name string, ks []tdx.Kline) error {
	data := [][]any{{"日期", "时间", "代码", "名称", "开盘(元)", "最高(元)", "最低(元)", "收盘(元)", "成交量(股)", "成交额(元)"}}
	for _, v := range ks {
		layout := protocol.TimeLayout
		clock := ""
		if len(v.Time) == len(time.DateOnly) {
			layout = time.DateOnly
		}
		at, err := time.Parse(layout, v.Time)
		if err != nil || at.Format(layout) != v.Time {
			return fmt.Errorf("export: 无效 K 线源时间 %q", v.Time)
		}
		if layout == protocol.TimeLayout {
			clock = at.Format("15:04")
		}
		var amount any = ""
		if v.Amount != nil {
			amount = *v.Amount
		}
		data = append(data, []any{
			at.Format(time.DateOnly),
			clock,
			code,
			name,
			v.Open,
			v.High,
			v.Low,
			v.Close,
			v.Volume,
			amount,
		})
	}

	return writeCSV(filename, data)
}

// TradesToCSV 接受统一 Client 的标准分笔，成交量按股输出；未知价格与成交额留空。
func TradesToCSV(filename string, ts []tdx.Trade) error {
	data := [][]any{{"日期", "时间", "价格(元)", "成交量(股)", "成交额(元)", "方向(0买,1卖,2中性)"}}
	for _, v := range ts {
		date := ""
		if v.Date != "" {
			at, err := time.Parse("20060102", v.Date)
			if err != nil || at.Format("20060102") != v.Date {
				return fmt.Errorf("export: 无效分笔源日期 %q", v.Date)
			}
			date = at.Format(time.DateOnly)
		}
		at, err := time.Parse("15:04:05", v.Time)
		if err != nil || at.Format("15:04:05") != v.Time {
			return fmt.Errorf("export: 无效分笔源时间 %q", v.Time)
		}
		var price, amount any = "", ""
		if v.Price != nil {
			price, amount = *v.Price, *v.Price*float64(v.Volume)
		}
		status := 2
		switch v.Direction {
		case 1:
			status = 0
		case -1:
			status = 1
		case 0:
		default:
			return fmt.Errorf("export: 无效分笔方向 %d", v.Direction)
		}
		data = append(data, []any{
			date,
			at.Format("15:04"),
			price,
			v.Volume,
			amount,
			status,
		})
	}
	return writeCSV(filename, data)
}
