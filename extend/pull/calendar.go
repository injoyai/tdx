package pull

import (
	"fmt"
	"sort"
	"time"
)

// CalendarPeriod 为日历对齐的日线合并周期。
type CalendarPeriod string

const (
	CalendarWeek    CalendarPeriod = "week" // ISO 周，周一开始
	CalendarMonth   CalendarPeriod = "month"
	CalendarQuarter CalendarPeriod = "quarter"
	CalendarYear    CalendarPeriod = "year"
)

// DayToCalendar 按指定时区的日历周/月/季/年合并，不填补缺失交易日。
// 输入无需排序；不修改输入。股本取最后一根，换手率累加，时间戳取最后实际记录。
// loc=nil 使用 time.Local。
func DayToCalendar(ks []*KlineDay, period CalendarPeriod, loc *time.Location) ([]*KlineDay, error) {
	if loc == nil {
		loc = time.Local
	}
	switch period {
	case CalendarWeek, CalendarMonth, CalendarQuarter, CalendarYear:
	default:
		return nil, fmt.Errorf("pull: 不支持的日历周期 %q", period)
	}
	input := append([]*KlineDay(nil), ks...)
	for _, k := range input {
		if k == nil {
			return nil, fmt.Errorf("pull: nil 日线")
		}
	}
	sort.SliceStable(input, func(i, j int) bool { return input[i].Unix < input[j].Unix })
	key := func(k *KlineDay) string {
		t := time.Unix(k.Unix, 0).In(loc)
		y, m, _ := t.Date()
		switch period {
		case CalendarWeek:
			y, w := t.ISOWeek()
			return fmt.Sprintf("%d-W%d", y, w)
		case CalendarMonth:
			return fmt.Sprintf("%d-%d", y, m)
		case CalendarQuarter:
			return fmt.Sprintf("%d-Q%d", y, (int(m)-1)/3)
		default:
			return fmt.Sprint(y)
		}
	}
	var out []*KlineDay
	for start := 0; start < len(input); {
		end, group := start+1, key(input[start])
		for end < len(input) && key(input[end]) == group {
			end++
		}
		k := *DayToPeriod(input[start:end], end-start)[0]
		out = append(out, &k)
		start = end
	}
	return out, nil
}

// TradingSession 表示交易所本地时间的一段连续交易时段，分钟数从 00:00 起算。
// EndMinute<=StartMinute 表示跨午夜（例如 21:00 到次日 02:30）；结束可为 1440。
// 开始时刻的独立记录（如 A 股集合竞价 09:30）单独保留，其余按收盘时刻 (start,end] 分桶。
type TradingSession struct {
	StartMinute int
	EndMinute   int
}

// MinuteToSessions 按时区和交易时段对齐 N 分钟线，不跨时段、不填造缺失分钟。
// 可用于 A 股、港美股、期货；调用方传实际时区/时段，夜盘按时段开始日定位。
// 输入无需排序，不修改输入；时段外数据报错，避免悄悄丢量。loc=nil 使用 time.Local。
func MinuteToSessions(ks []*KlineMinute, n int, loc *time.Location, sessions []TradingSession) ([]*KlineMinute, error) {
	if n <= 0 {
		return nil, fmt.Errorf("pull: 分钟周期必须大于零")
	}
	if loc == nil {
		loc = time.Local
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("pull: 交易时段不能为空")
	}
	var occupied [1440]bool
	for _, s := range sessions {
		if s.StartMinute < 0 || s.StartMinute >= 1440 || s.EndMinute < 0 || s.EndMinute > 1440 || s.StartMinute == s.EndMinute {
			return nil, fmt.Errorf("pull: 无效交易时段 %+v", s)
		}
		end := s.EndMinute
		if end < s.StartMinute {
			end += 1440
		}
		for m := s.StartMinute; m <= end; m++ {
			// 全天时段的首尾是同一分钟，只检查一次，避免将自身误判为重叠。
			if m == end && m-s.StartMinute == 1440 {
				continue
			}
			if occupied[m%1440] {
				return nil, fmt.Errorf("pull: 交易时段重叠")
			}
			occupied[m%1440] = true
		}
	}
	input := append([]*KlineMinute(nil), ks...)
	for _, k := range input {
		if k == nil {
			return nil, fmt.Errorf("pull: nil 分钟线")
		}
	}
	sort.SliceStable(input, func(i, j int) bool { return input[i].Unix < input[j].Unix })
	type bucket struct {
		anchor         int64
		session, index int
	}
	var previous bucket
	var out []*KlineMinute
	for _, k := range input {
		t := time.Unix(k.Unix, 0).In(loc)
		var current bucket
		found := false
		for i, s := range sessions {
			for back := 0; back <= 1; back++ {
				date := t.AddDate(0, 0, -back)
				begin := time.Date(date.Year(), date.Month(), date.Day(), s.StartMinute/60, s.StartMinute%60, 0, 0, loc)
				endMinute := s.EndMinute
				if endMinute < s.StartMinute {
					endMinute += 1440
				}
				end := time.Date(date.Year(), date.Month(), date.Day(), endMinute/60, endMinute%60, 0, 0, loc)
				if t.Before(begin) || t.After(end) {
					continue
				}
				elapsed := int64(t.Sub(begin) / time.Second)
				index := -1 // 开始时刻单独保留
				if elapsed > 0 {
					index = int((elapsed - 1) / (int64(n) * 60))
				}
				current, found = bucket{begin.Unix(), i, index}, true
				break
			}
			if found {
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("pull: %s 不在配置的交易时段内", t.Format(time.RFC3339))
		}
		if len(out) == 0 || current != previous {
			copy := *k
			out = append(out, &copy)
		} else {
			last := out[len(out)-1]
			last.High, last.Low = max(last.High, k.High), min(last.Low, k.Low)
			last.Close, last.Unix = k.Close, k.Unix
			last.Volume += k.Volume
			last.Amount += k.Amount
		}
		previous = current
	}
	return out, nil
}
