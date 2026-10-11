package market

import "github.com/injoyai/tdx/v2/extend/pull"

// saveDays 排除 Until 返回的界外停止记录，仅覆盖实际返回的尾部。
func saveDays(s *pull.Service, code pull.Code, from int64, rows []*pull.KlineDay) error {
	var out []*pull.KlineDay
	var writeFrom int64
	for _, k := range rows {
		if k.Unix < from {
			continue
		}
		if len(out) == 0 || k.Unix < writeFrom {
			writeFrom = k.Unix
		}
		out = append(out, k)
	}
	return s.SaveDay(code, writeFrom, out)
}
