package market

import (
	"time"

	"github.com/injoyai/tdx/extend/pull"
)

type minuteTarget struct {
	year       int
	from       int64
	historical bool
}

// pullMinutes 所有缺失年份共用一次从新到旧的扫描；扫描成功后再按年提交。
// 完成指已读完服务器可提供的区间，不保证服务器保留了上市以来的全部历史。
func pullMinutes(s *pull.Service, code pull.Code, now time.Time,
	fetch func(time.Time) ([]*pull.KlineMinute, error)) error {
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
		if historical {
			done, err := s.MinYearComplete(code, year, from.Unix())
			if err != nil {
				return err
			}
			if done {
				continue
			}
		} else {
			last, err := s.LastMinUnix(code, year)
			if err != nil {
				return err
			}
			if last > from.Unix() {
				from = time.Unix(last, 0)
			}
		}
		targets = append(targets, minuteTarget{year, from.Unix(), historical})
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
		if target.historical {
			err = s.SaveMinComplete(code, target.year, target.from, out)
		} else {
			err = s.SaveMin(code, target.year, target.from, out)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
