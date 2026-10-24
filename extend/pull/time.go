package pull

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// parseTime 仅用 UTC 容器进行无时区的日历计算，不把结果当作源时间戳。
func parseTime(value, layout string) (time.Time, error) {
	t, err := time.Parse(layout, value)
	if err != nil || t.Format(layout) != value {
		return time.Time{}, fmt.Errorf("pull: 无效源时间 %q，格式须为 %s", value, layout)
	}
	return t, nil
}

func validateRange(start, end, layout string) error {
	for _, value := range []string{start, end} {
		if value != "" {
			if _, err := parseTime(value, layout); err != nil {
				return err
			}
		}
	}
	if start != "" && end != "" && start > end {
		return fmt.Errorf("pull: 起始时间晚于结束时间")
	}
	return nil
}

// Timestamp 仅在调用方已确认来源时区后转换；不会改变存储的源时间。
func (k KlineMinute) Timestamp(loc *time.Location) (int64, error) {
	t, err := protocol.ParseSourceTime(k.Time, loc)
	if err != nil {
		return 0, err
	}
	return t.Unix(), nil
}

func validateDays(from string, rows []*KlineDay) error {
	if err := validateRange(from, "", time.DateOnly); err != nil {
		return err
	}
	for _, k := range rows {
		if k == nil {
			return fmt.Errorf("pull: nil 日线")
		}
		if _, err := parseTime(k.Time, time.DateOnly); err != nil {
			return err
		}
		if err := validateRange(k.TradingDate, "", time.DateOnly); err != nil {
			return err
		}
	}
	return nil
}

func validateMinutes(year int, from string, rows []*KlineMinute) error {
	if err := validateRange(from, "", protocol.TimeLayout); err != nil {
		return err
	}
	for _, k := range rows {
		if k == nil {
			return fmt.Errorf("pull: nil 分钟线")
		}
		t, err := parseTime(k.Time, protocol.TimeLayout)
		if err != nil {
			return err
		}
		if t.Year() != year {
			return fmt.Errorf("pull: 源时间 %s 不属于 %d 年", k.Time, year)
		}
		if err := validateRange(k.TradingDate, "", time.DateOnly); err != nil {
			return err
		}
		if k.Unix == nil && k.TimeZone == "" {
			continue
		}
		if k.TimeZone == "Local" {
			return fmt.Errorf("pull: 来源时区须显式指定，不能使用主机 Local")
		}
		if k.Unix == nil || k.TimeZone == "" {
			return fmt.Errorf("pull: Unix 和 TimeZone 须同时提供")
		}
		loc := protocol.Location()
		if k.TimeZone != loc.String() {
			loc, err = time.LoadLocation(k.TimeZone)
			if err != nil {
				return err
			}
		}
		stamp, err := k.Timestamp(loc)
		if err != nil {
			return fmt.Errorf("pull: %w", err)
		}
		if stamp != *k.Unix {
			return fmt.Errorf("pull: 时间戳与源时间 %s、时区 %s 不一致", k.Time, k.TimeZone)
		}
	}
	return nil
}
