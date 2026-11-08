package metadata

import (
	"fmt"
	"iter"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

type Workday struct {
	days     map[string]bool
	snapshot model.WorkdaySnapshot
}

// NewWorkday validates and copies a calendar. Absence outside its coverage is unknown.
func NewWorkday(snapshot model.WorkdaySnapshot) (*Workday, error) {
	if snapshot.Market != protocol.MarketAStock {
		return nil, fmt.Errorf("workday: unsupported calendar market %s", snapshot.Market)
	}
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	copy := snapshot
	copy.Dates = append([]string(nil), snapshot.Dates...)
	w := &Workday{days: map[string]bool{}, snapshot: copy}
	for _, date := range copy.Dates {
		w.days[date] = true
	}
	return w, nil
}

// Status returns (observedTradingDay, coveredBySource). Missing dates are not proof of an official holiday.
func (w *Workday) Status(at time.Time) (bool, bool) {
	date := at.In(protocol.Location()).Format(time.DateOnly)
	return w.days[date], w.snapshot.From != "" && date >= w.snapshot.From && date <= w.snapshot.Through
}

// Snapshot returns an independent copy for publication or serialization.
func (w *Workday) Snapshot() model.WorkdaySnapshot {
	copy := w.snapshot
	copy.Dates = append([]string(nil), copy.Dates...)
	return copy
}

// Is 是否是工作日
func (w *Workday) Is(t time.Time) bool {
	t = t.In(protocol.Location())
	return w.days[t.Format(time.DateOnly)]
}

// TodayIs 今天是否是工作日
func (w *Workday) TodayIs() bool {
	return w.Is(time.Now())
}

// RangeYear 遍历一年的所有工作日
func (w *Workday) RangeYear(year int, f func(t time.Time) bool) {
	w.Range(
		time.Date(year, 1, 1, 0, 0, 0, 0, protocol.Location()),
		time.Date(year, 12, 31, 0, 0, 0, 1, protocol.Location()),
		f,
	)
}

// Range 遍历指定范围的工作日,推荐start带上时间15:00,这样当天小于15点不会触发
func (w *Workday) Range(start, end time.Time, f func(t time.Time) bool) {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	for ; start.Before(end); start = start.Add(time.Hour * 24) {
		if w.Is(start) {
			if !f(start) {
				return
			}
		}
	}
}

func (w *Workday) IterYear(year int, desc ...bool) iter.Seq[time.Time] {
	return w.Iter(
		time.Date(year, 1, 1, 0, 0, 0, 0, protocol.Location()),
		time.Date(year, 12, 31, 0, 0, 0, 1, protocol.Location()),
		desc...,
	)
}

// Iter 遍历 [start,end) 内从 start 起每隔 24 小时的工作日；倒序仅改变顺序。
func (w *Workday) Iter(start, end time.Time, desc ...bool) iter.Seq[time.Time] {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	if len(desc) > 0 && desc[0] {
		//倒序遍历
		return func(yield func(time.Time) bool) {
			if !start.Before(end) {
				return
			}
			last := start.Add((end.Sub(start) - 1) / (24 * time.Hour) * (24 * time.Hour))
			for at := last; !at.Before(start); at = at.Add(-24 * time.Hour) {
				if w.Is(at) {
					if !yield(at) {
						return
					}
				}
			}
		}
	}
	//正序遍历
	return func(yield func(time.Time) bool) {
		for at := start; at.Before(end); at = at.Add(time.Hour * 24) {
			if w.Is(at) {
				if !yield(at) {
					return
				}
			}
		}
	}
}
