// Package export 将标准协议 K 线与分笔导出为带 UTF-8 BOM 的 CSV 文件。
package export

import (
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func KlinesToCSV(filename string, code, name string, ks protocol.Klines) error {
	data := [][]any{{"日期", "时间", "代码", "名称", "开盘(元)", "最高(元)", "最低(元)", "收盘(元)", "成交量(股)", "成交额(元)"}}
	for _, v := range ks {
		data = append(data, []any{
			v.Time.Format(time.DateOnly),
			v.Time.Format("15:04"),
			code,
			name,
			v.Open,
			v.High,
			v.Low,
			v.Close,
			v.Volume * 100,
			v.Amount,
		})
	}

	buf, err := toCSV(data)
	if err != nil {
		return err
	}

	return writeFile(filename, buf)
}

func TradesToCSV(filename string, ts protocol.Trades) error {
	data := [][]any{{"日期", "时间", "价格(元)", "成交量(股)", "成交额(元)", "方向(0买,1卖,2中性)"}}
	for _, v := range ts {
		data = append(data, []any{
			v.Time.Format(time.DateOnly),
			v.Time.Format("15:04"),
			v.Price,
			v.Volume * 100,
			v.Amount(),
			v.Status,
		})
	}
	buf, err := toCSV(data)
	if err != nil {
		return err
	}
	return writeFile(filename, buf)
}
