package pull

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// Schedule defines the local daily update boundary; it does not interpret source timestamps.
type Schedule struct {
	TimeZone  string
	CloseTime string        // HH:MM
	Delay     time.Duration // zero means no delay for an explicit schedule
}

func (s Schedule) boundary(now time.Time) (time.Time, error) {
	if s.TimeZone == "" || s.Delay < 0 {
		return time.Time{}, fmt.Errorf("pull: 无效调度配置")
	}
	loc, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return time.Time{}, err
	}
	clock, err := time.Parse("15:04", s.CloseTime)
	if err != nil || clock.Format("15:04") != s.CloseTime {
		return time.Time{}, fmt.Errorf("pull: 无效收盘时间 %q", s.CloseTime)
	}
	clockOffset := time.Duration(clock.Hour())*time.Hour + time.Duration(clock.Minute())*time.Minute
	if s.Delay >= 24*time.Hour-clockOffset {
		return time.Time{}, fmt.Errorf("pull: 收盘时间加延迟必须早于当地次日零点")
	}
	now = now.In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), clock.Hour(), clock.Minute(), 0, 0, loc).Add(s.Delay), nil
}

func defaultSchedule(market protocol.Market) (Schedule, error) {
	info, known := market.Info()
	if !known {
		return Schedule{}, fmt.Errorf("pull: 自定义市场 %s 需要显式配置 Schedule", market)
	}
	return Schedule{TimeZone: info.TimeZone, CloseTime: info.CloseTime, Delay: 5 * time.Minute}, nil
}

func (s *Service) schedule(u Unit) (Schedule, error) {
	if value, ok := s.cfg.Schedules[protocol.Market(u.Name())]; ok {
		return value, nil
	}
	if _, known := protocol.Market(u.Name()).Info(); known {
		return defaultSchedule(protocol.Market(u.Name()))
	}
	if unit, ok := u.(interface{ Schedule() Schedule }); ok {
		return unit.Schedule(), nil
	}
	return Schedule{}, fmt.Errorf("pull: 自定义市场 %s 需要显式配置 Schedule", u.Name())
}

func marketUpdateTime(market protocol.Market, now time.Time) (time.Time, error) {
	schedule, err := defaultSchedule(market)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.boundary(now)
}
