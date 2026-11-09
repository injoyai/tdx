package pull

import "github.com/injoyai/tdx/v2/extend/model"

// 任一组成记录金额未知时，合计仍未知；不改写输入指针。
func sumAmount(a, b *float64) *float64 {
	if a == nil || b == nil {
		return nil
	}
	v := *a + *b
	return &v
}

// 周期合并只计算原始浮点字段，避免经过整数厘损失港美股等市场的价格精度。

// DayToPeriod 将日线按固定 N 根分块合并（非日历对齐）；n<=1 原样返回。
func DayToPeriod(ks []*model.KlineDay, n int) []*model.KlineDay {
	if n <= 1 {
		return ks
	}
	var out []*model.KlineDay
	for start := 0; start < len(ks); {
		end := start + min(n, len(ks)-start)
		k := *ks[start]
		for _, v := range ks[start+1 : end] {
			if k.Open == 0 {
				k.Open = v.Open
			}
			k.High, k.Low = max(k.High, v.High), min(k.Low, v.Low)
			k.Time, k.Close = v.Time, v.Close
			k.Volume += v.Volume
			k.Amount = sumAmount(k.Amount, v.Amount)
		}
		out = append(out, &k)
		start = end
	}
	return out
}

// MinuteToPeriod 按固定 N 根合并，不对齐交易日或时段；n<=1 原样返回。
// 时段对齐请用 MinuteToSessions。
func MinuteToPeriod(ks []*model.KlineMinute, n int) []*model.KlineMinute {
	if n <= 1 {
		return ks
	}
	var out []*model.KlineMinute
	for start := 0; start < len(ks); {
		end := start + min(n, len(ks)-start)
		k := *ks[start]
		for _, v := range ks[start+1 : end] {
			if k.Open == 0 {
				k.Open = v.Open
			}
			k.High, k.Low = max(k.High, v.High), min(k.Low, v.Low)
			k.Time, k.Close = v.Time, v.Close
			k.Volume += v.Volume
			k.Amount = sumAmount(k.Amount, v.Amount)
			if v.Source != "" {
				k.Source = v.Source
			}
		}
		out = append(out, &k)
		start = end
	}
	return out
}
