package market

import "github.com/injoyai/tdx/v2/extend/pull"

// saveDays 排除 Until 返回的界外停止记录，仅覆盖实际返回的尾部。
func saveDays(s *pull.Service, code pull.Code, from string, rows []*pull.KlineDay) error {
	var out []*pull.KlineDay
	var writeFrom string
	for _, k := range rows {
		if k.Time < from {
			continue
		}
		if len(out) == 0 || k.Time < writeFrom {
			writeFrom = k.Time
		}
		out = append(out, k)
	}
	return s.SaveDay(code, writeFrom, out)
}
