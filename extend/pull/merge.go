package pull

// 周期合并只计算原始浮点字段，避免经过整数厘损失港美股等市场的价格精度。

// DayToPeriod 将日线按固定 N 根分块合并（非日历对齐）；n<=1 原样返回。
func DayToPeriod(ks []*KlineDay, n int) []*KlineDay {
	if n <= 1 {
		return ks
	}
	var out []*KlineDay
	for start := 0; start < len(ks); {
		end := start + min(n, len(ks)-start)
		k := *ks[start]
		for _, v := range ks[start+1 : end] {
			if k.Open == 0 {
				k.Open = v.Open
			}
			k.High, k.Low = max(k.High, v.High), min(k.Low, v.Low)
			k.Unix, k.Close = v.Unix, v.Close
			k.Volume += v.Volume
			k.Amount += v.Amount
			k.AmountUnknown = k.AmountUnknown || v.AmountUnknown
			k.Turnover += v.Turnover
			k.FloatStock, k.TotalStock = v.FloatStock, v.TotalStock
		}
		if k.AmountUnknown {
			k.Amount = 0
		}
		out = append(out, &k)
		start = end
	}
	return out
}

// MinuteToPeriod 按固定 N 根合并，不对齐交易日或时段；n<=1 原样返回。
// 时段对齐请用 MinuteToSessions。
func MinuteToPeriod(ks []*KlineMinute, n int) []*KlineMinute {
	if n <= 1 {
		return ks
	}
	var out []*KlineMinute
	for start := 0; start < len(ks); {
		end := start + min(n, len(ks)-start)
		k := *ks[start]
		for _, v := range ks[start+1 : end] {
			if k.Open == 0 {
				k.Open = v.Open
			}
			k.High, k.Low = max(k.High, v.High), min(k.Low, v.Low)
			k.Unix, k.Close = v.Unix, v.Close
			k.Volume += v.Volume
			k.Amount += v.Amount
			k.AmountUnknown = k.AmountUnknown || v.AmountUnknown
			if v.Source != "" {
				k.Source = v.Source
			}
		}
		if k.AmountUnknown {
			k.Amount = 0
		}
		out = append(out, &k)
		start = end
	}
	return out
}
