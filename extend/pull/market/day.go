package market

import (
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
)

// saveDays 排除 Until 返回的界外停止记录，仅覆盖实际返回的尾部。
func saveDays(s *pull.Service, code model.Code, from string, rows []*model.KlineDay) error {
	_, err := saveDaysResult(s, code, from, rows)
	return err
}

func saveDaysResult(s *pull.Service, code model.Code, from string, rows []*model.KlineDay) (pull.FetchResult, error) {
	result := pull.FetchResult{Known: true, Fetched: len(rows)}
	var out []*model.KlineDay
	var writeFrom string
	for _, k := range rows {
		result.LatestTime = max(result.LatestTime, k.Time)
		if k.Time < from {
			continue
		}
		if len(out) == 0 || k.Time < writeFrom {
			writeFrom = k.Time
		}
		out = append(out, k)
	}
	err := s.SaveDay(code, writeFrom, out)
	if err == nil {
		result.Saved = len(out)
	}
	return result, err
}
