package pull

import (
	"time"

	"github.com/injoyai/tdx/protocol"
)

// 周期合并：只存日线 + 1分钟线，其余周期（5/15/30/60分钟、周/月/季/年）由二者派生，
// 纯内存计算、不落盘。

// yuan 四舍五入的元→厘转换（protocol.Yuan 为直接截断，往返会差 ±1 厘）。
func yuan(f float64) protocol.Price {
	return protocol.Price(f*1000 + 0.5)
}

// toProtocol 将日线转成 protocol.Klines 便于复用合并逻辑（元→厘）。
func toProtocol(ks []*KlineDay) protocol.Klines {
	out := make(protocol.Klines, 0, len(ks))
	for _, k := range ks {
		out = append(out, &protocol.Kline{
			Open:   yuan(k.Open),
			High:   yuan(k.High),
			Low:    yuan(k.Low),
			Close:  yuan(k.Close),
			Volume: k.Volume,
			Amount: yuan(k.Amount),
			Time:   time.Unix(k.Unix, 0),
		})
	}
	return out
}

func fromProtocol(ks protocol.Klines) []*KlineDay {
	out := make([]*KlineDay, 0, len(ks))
	for _, k := range ks {
		out = append(out, &KlineDay{
			Unix:   k.Time.Unix(),
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: k.Volume,
			Amount: k.Amount.Float64(),
		})
	}
	return out
}

// DayToPeriod 将日线按固定 N 根分块合并（近似 N 日周期，如 5≈周、20≈月、60≈季、250≈年，
// 非日历对齐；首块从传入序列开头起算）。
// n<=1 时原样返回。
func DayToPeriod(ks []*KlineDay, n int) []*KlineDay {
	if n <= 1 {
		return ks
	}
	out := fromProtocol(toProtocol(ks).Merge(n))
	for i, k := range out {
		end := min((i+1)*n, len(ks))
		k.FloatStock, k.TotalStock = ks[end-1].FloatStock, ks[end-1].TotalStock
		for _, day := range ks[i*n : end] {
			k.Turnover += day.Turnover
		}
	}
	return out
}

// MinuteToPeriod 按固定 N 根合并；不对齐交易日或时段。时段对齐请用 MinuteToSessions。
// n<=1 时原样返回。
func MinuteToPeriod(ks []*KlineMinute, n int) []*KlineMinute {
	if n <= 1 {
		return ks
	}
	// 先将分钟线转为统一 protocol.Klines（元→厘）
	in := make(protocol.Klines, 0, len(ks))
	for _, k := range ks {
		in = append(in, &protocol.Kline{
			Open:   yuan(k.Open),
			High:   yuan(k.High),
			Low:    yuan(k.Low),
			Close:  yuan(k.Close),
			Volume: k.Volume,
			Amount: yuan(k.Amount),
			Time:   time.Unix(k.Unix, 0),
		})
	}
	merged := in.Merge(n)
	out := make([]*KlineMinute, 0, len(merged))
	for i, k := range merged {
		source := ""
		for _, input := range ks[i*n : min((i+1)*n, len(ks))] {
			if input.Source != "" {
				source = input.Source
			}
		}
		out = append(out, &KlineMinute{
			Unix:   k.Time.Unix(),
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Volume: k.Volume,
			Amount: k.Amount.Float64(),
			Source: source,
		})
	}
	return out
}
