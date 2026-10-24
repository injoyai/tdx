// Package export 将标准协议 K 线与分笔导出为带 UTF-8 BOM 的 CSV 文件。
package export

import (
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func KlinesToCSV(filename string, code, name string, ks protocol.Klines) error {
	data := [][]any{{"日期", "时间", "代码", "名称", "开盘", "最高", "最低", "收盘", "总手", "金额"}}
	for _, v := range ks {
		data = append(data, []any{
			v.Time.Format("20060102"),
			v.Time.Format("15:04"),
			code,
			name,
			v.Open.Float64(),
			v.High.Float64(),
			v.Low.Float64(),
			v.Close.Float64(),
			v.Volume,
			v.Amount.Float64(),
		})
	}

	buf, err := toCSV(data)
	if err != nil {
		return err
	}

	return writeFile(filename, buf)
}

func TradesToCSV(filename string, ts protocol.Trades) error {
	data := [][]any{{"日期", "时间", "价格", "成交量(手)", "成交额", "方向(0买,1卖)"}}
	for _, v := range ts {
		data = append(data, []any{
			v.Time.Format(time.DateOnly),
			v.Time.Format("15:04"),
			v.Price.Float64(),
			v.Volume,
			v.Amount().Float64(),
			v.Status,
		})
	}
	buf, err := toCSV(data)
	if err != nil {
		return err
	}
	return writeFile(filename, buf)
}
