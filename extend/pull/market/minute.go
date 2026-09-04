package market

import (
	"errors"
	"sync"
	"time"

	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

type minuteTarget struct {
	year       int
	from       int64
	historical bool
	through    int64
}

// pullMinutes 所有缺失年份共用一次从新到旧的扫描；扫描成功后再按年提交。
// 完成指已读完服务器可提供的区间，不保证服务器保留了上市以来的全部历史。
func pullMinutes(s *pull.Service, code pull.Code, now time.Time,
	fetch func(time.Time) ([]*pull.KlineMinute, error), supplement *minuteFallback) error {
	start := s.Start()
	if start.After(now) {
		return nil
	}
	var targets []minuteTarget
	var earliest time.Time
	for year := start.Year(); year <= now.Year(); year++ {
		from := time.Date(year, 1, 1, 0, 0, 0, 0, start.Location())
		if start.After(from) {
			from = start
		}
		historical := year < now.Year()
		through := time.Date(year+1, 1, 1, 0, 0, 0, 0, start.Location()).Unix()
		if !historical {
			through = now.Unix()
		}
		coverage, err := s.QueryMinCoverage(code, year)
		if err != nil {
			return err
		}
		if historical {
			if coverage != nil && coverage.From <= from.Unix() &&
				(!coverage.TradeFallback || coverage.Through >= through) &&
				(supplement == nil || coverage.TradeFallback) {
				continue
			}
		} else {
			last, err := s.LastMinUnix(code, year)
			if err != nil {
				return err
			}
			if supplement != nil {
				// 旧标记或未完成的当年仍需补早期历史，不能只从最新一根继续。
				if coverage == nil || !coverage.TradeFallback || coverage.From > from.Unix() {
					last = 0
				} else {
					last = min(last, coverage.Through)
				}
			}
			if last > from.Unix() {
				from = time.Unix(last, 0)
			}
		}
		targets = append(targets, minuteTarget{year: year, from: from.Unix(), historical: historical, through: through})
		if earliest.IsZero() || from.Before(earliest) {
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
	boundary := minuteDay(now, start.Location())
	for _, k := range ks {
		day := minuteDay(time.Unix(k.Unix, 0), start.Location())
		if day.Before(boundary) {
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
			daysOnce.Do(func() { days, daysErr = originalDays(earliest) })
			return days, daysErr
		}}
		var merged []*pull.KlineMinute
		for _, target := range targets {
			var yearNative []*pull.KlineMinute
			for _, k := range ks {
				if k.Unix >= target.from && k.Unix < target.through {
					yearNative = append(yearNative, k)
				}
			}
			out, err := supplement.fill(s, code, time.Unix(target.from, 0),
				time.Unix(target.through, 0), boundary, yearNative)
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
		year := time.Unix(k.Unix, 0).In(start.Location()).Year()
		byYear[year] = append(byYear[year], k)
	}
	for _, target := range targets {
		var out []*pull.KlineMinute
		for _, k := range byYear[target.year] {
			if k.Unix >= target.from {
				out = append(out, k)
			}
		}
		if supplement != nil && !fallbackFailed[target.year] {
			err = s.SaveMinFallbackComplete(code, target.year, target.from, target.through, out)
		} else if target.historical && supplement == nil {
			err = s.SaveMinComplete(code, target.year, target.from, out)
		} else {
			err = s.SaveMin(code, target.year, target.from, out)
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
