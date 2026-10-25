package market

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

type minuteTarget struct {
	year       int
	from       string
	historical bool
	through    string
}

// pullMinutes 所有缺失年份共用一次从新到旧的扫描；扫描成功后再按年提交。
// 完成指已读完服务器可提供的区间，不保证服务器保留了上市以来的全部历史。
func pullMinutes(s *pull.Service, code pull.Code, now time.Time,
	fetch func(string) ([]*pull.KlineMinute, error), supplement *minuteFallback) error {
	start := s.Start()
	now = now.In(start.Location())
	info, known := code.Market.Info()
	if start.After(now) && known && info.Protocol == protocol.ProtocolStandard {
		return nil
	}
	var targets []minuteTarget
	var earliest string
	for year := start.Year(); year <= max(start.Year(), now.Year()); year++ {
		from := max(fmt.Sprintf("%04d-01-01 00:00", year), start.Format(protocol.TimeLayout))
		historical := year < now.Year()
		through := fmt.Sprintf("%04d-01-01 00:00", year+1)
		if !historical {
			through = now.Add(time.Minute).Format(protocol.TimeLayout)
		}
		coverage, err := s.QueryMinuteCoverage(code, year)
		if err != nil {
			return err
		}
		if historical {
			if coverage != nil && coverage.From <= from &&
				(!coverage.TradeFallback || coverage.Through >= through) &&
				(supplement == nil || coverage.TradeFallback) {
				continue
			}
			if supplement == nil {
				empty, err := s.MinuteYearRecentlyEmpty(code, year, from, now)
				if err != nil {
					return err
				}
				if empty {
					continue
				}
			}
		} else {
			last, err := s.LastMinuteTime(code, year)
			if err != nil {
				return err
			}
			if supplement != nil {
				// 旧标记或未完成的当年仍需补早期历史，不能只从最新一根继续。
				if coverage == nil || !coverage.TradeFallback || coverage.From > from {
					last = ""
				} else {
					last = min(last, coverage.Through)
				}
			}
			if last > from {
				from = last
			}
		}
		targets = append(targets, minuteTarget{year: year, from: from, historical: historical, through: through})
		if earliest == "" || from < earliest {
			earliest = from
		}
	}
	if len(targets) == 0 {
		return nil
	}
	ks, err := fetch(earliest)
	if err != nil {
		return err
	}
	planned := make(map[int]bool, len(targets))
	for _, target := range targets {
		planned[target.year] = true
	}
	boundary := now.Format(time.DateOnly)
	for _, k := range ks {
		if k == nil {
			return fmt.Errorf("pull: nil 分钟线")
		}
		at, err := time.Parse(protocol.TimeLayout, k.Time)
		if err != nil || at.Format(protocol.TimeLayout) != k.Time {
			return fmt.Errorf("pull: 无效源时间 %q", k.Time)
		}
		// 来源时区未知时，服务器日期可能已进入下一年；按实际返回的年份落库。
		if at.Year() > now.Year() && !planned[at.Year()] && k.Time >= start.Format(protocol.TimeLayout) {
			from := max(fmt.Sprintf("%04d-01-01 00:00", at.Year()), start.Format(protocol.TimeLayout))
			last, err := s.LastMinuteTime(code, at.Year())
			if err != nil {
				return err
			}
			from = max(from, last)
			targets = append(targets, minuteTarget{year: at.Year(), from: from, through: fmt.Sprintf("%04d-01-01 00:00", at.Year()+1)})
			planned[at.Year()] = true
		}
		day := k.Time[:10]
		if day < boundary {
			boundary = day
		}
	}
	var fallbackErrs []error
	fallbackFailed := make(map[int]bool)
	if supplement != nil {
		// 各年份独立处理，但日线候选只请求一次。
		originalDays := supplement.days
		var daysOnce sync.Once
		var days protocol.Klines
		var daysErr error
		supplement = &minuteFallback{trades: supplement.trades, days: func(time.Time) (protocol.Klines, error) {
			daysOnce.Do(func() {
				at, err := time.ParseInLocation(protocol.TimeLayout, earliest, protocol.Location())
				if err != nil {
					daysErr = err
					return
				}
				days, daysErr = originalDays(at)
			})
			return days, daysErr
		}}
		var merged []*pull.KlineMinute
		for _, target := range targets {
			var yearNative []*pull.KlineMinute
			for _, k := range ks {
				if k.Time >= target.from && k.Time < target.through {
					yearNative = append(yearNative, k)
				}
			}
			out, err := supplement.fill(s, code, target.from, target.through, boundary, yearNative)
			if err != nil {
				fallbackErrs = append(fallbackErrs, err)
				fallbackFailed[target.year] = true
			}
			merged = append(merged, out...)
		}
		ks = merged
	}
	var saveErrors []error
	byYear := make(map[int][]*pull.KlineMinute)
	for _, k := range ks {
		var year int
		fmt.Sscanf(k.Time[:4], "%d", &year)
		byYear[year] = append(byYear[year], k)
	}
	for _, target := range targets {
		var out []*pull.KlineMinute
		for _, k := range byYear[target.year] {
			if k.Time >= target.from {
				out = append(out, k)
			}
		}
		if len(out) == 0 {
			if target.historical && supplement == nil && len(ks) > 0 {
				if err := s.SaveMinuteEmptyScan(code, target.year, target.from, now); err != nil {
					return err
				}
			}
			continue
		}
		if supplement != nil && !fallbackFailed[target.year] {
			err = s.SaveMinuteFallbackComplete(code, target.year, target.from, target.through, out)
		} else if target.historical && supplement == nil {
			err = s.SaveMinuteComplete(code, target.year, target.from, out)
		} else {
			// 保留服务器滚动历史窗口之前的本地记录。
			writeFrom := out[0].Time
			for _, k := range out[1:] {
				writeFrom = min(writeFrom, k.Time)
			}
			err = s.SaveMinute(code, target.year, writeFrom, out)
		}
		if err != nil {
			if supplement == nil {
				return err
			}
			saveErrors = append(saveErrors, err)
		}
	}
	return errors.Join(append(saveErrors, fallbackErrs...)...)
}
