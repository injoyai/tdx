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

// DayToCalendar 按源日期的日历周/月/季/年合并，不填补缺失交易日。
// 输入无需排序；不修改输入。股本取最后一根，换手率累加，源日期取最后实际记录。
func DayToCalendar(ks []*KlineDay, period CalendarPeriod) ([]*KlineDay, error) {
	switch period {
	case CalendarWeek, CalendarMonth, CalendarQuarter, CalendarYear:
	default:
		return nil, fmt.Errorf("pull: 不支持的日历周期 %q", period)
	}
	if err := validateDays("", ks); err != nil {
		return nil, err
	}
	input := append([]*KlineDay(nil), ks...)
	sort.SliceStable(input, func(i, j int) bool { return input[i].Time < input[j].Time })
	key := func(k *KlineDay) string {
		t, _ := parseTime(k.Time, time.DateOnly)
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

// MinuteToSessions 按源日期时分和交易时段对齐 N 分钟线，不跨时段、不填造缺失分钟。
// 调用方传入对应来源的钟面时段；夜盘按时段开始日分桶，不推断交易日。
// 输入无需排序，不修改输入；时段外数据报错，避免悄悄丢量。
func MinuteToSessions(ks []*KlineMinute, n int, sessions []TradingSession) ([]*KlineMinute, error) {
	if n <= 0 {
		return nil, fmt.Errorf("pull: 分钟周期必须大于零")
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
	for _, k := range ks {
		if k == nil {
			return nil, fmt.Errorf("pull: nil 分钟线")
		}
		if _, err := parseTime(k.Time, "2006-01-02 15:04"); err != nil {
			return nil, err
		}
	}
	input := append([]*KlineMinute(nil), ks...)
	sort.SliceStable(input, func(i, j int) bool { return input[i].Time < input[j].Time })
	type bucket struct {
		anchor         string
		session, index int
	}
	var previous bucket
	var out []*KlineMinute
	for _, k := range input {
		t, _ := parseTime(k.Time, "2006-01-02 15:04")
		var current bucket
		found := false
		for i, s := range sessions {
			for back := 0; back <= 1; back++ {
				date := t.AddDate(0, 0, -back)
				begin := time.Date(date.Year(), date.Month(), date.Day(), s.StartMinute/60, s.StartMinute%60, 0, 0, time.UTC)
				endMinute := s.EndMinute
				if endMinute < s.StartMinute {
					endMinute += 1440
				}
				end := time.Date(date.Year(), date.Month(), date.Day(), endMinute/60, endMinute%60, 0, 0, time.UTC)
				if t.Before(begin) || t.After(end) {
					continue
				}
				elapsed := int64(t.Sub(begin) / time.Second)
				index := -1 // 开始时刻单独保留
				if elapsed > 0 {
					index = int((elapsed - 1) / (int64(n) * 60))
				}
				current, found = bucket{anchor: begin.Format("2006-01-02 15:04"), session: i, index: index}, true
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
			last.Close, last.Time = k.Close, k.Time
			last.Volume += k.Volume
			last.Amount += k.Amount
			if k.Source != "" {
				last.Source = k.Source
			}
		}
		previous = current
	}
	return out, nil
}
