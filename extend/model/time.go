package model

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// parseTime 仅用 UTC 容器校验钟面值，不把结果当作源时间戳。
func parseTime(value, layout string) (time.Time, error) {
	t, err := time.Parse(layout, value)
	if err != nil || t.Format(layout) != value {
		return time.Time{}, fmt.Errorf("model: 无效源时间 %q，格式须为 %s", value, layout)
	}
	return t, nil
}

// ValidateSourceTime 校验源日期／日期时分，不解释来源时区。
func ValidateSourceTime(value, layout string) error {
	_, err := parseTime(value, layout)
	return err
}

// ValidateTimeRange 校验包含可选端点的源时间区间，空端点表示不限范围。
func ValidateTimeRange(start, end, layout string) error {
	for _, value := range []string{start, end} {
		if value != "" {
			if err := ValidateSourceTime(value, layout); err != nil {
				return err
			}
		}
	}
	if start != "" && end != "" && start > end {
		return fmt.Errorf("model: 起始时间晚于结束时间")
	}
	return nil
}

// ValidateDays 校验日线及可选写入边界，拒绝空记录和非法日期。
func ValidateDays(from string, rows []*KlineDay) error {
	if err := ValidateTimeRange(from, "", time.DateOnly); err != nil {
		return err
	}
	for _, k := range rows {
		if k == nil {
			return fmt.Errorf("model: nil 日线")
		}
		if err := ValidateSourceTime(k.Time, time.DateOnly); err != nil {
			return err
		}
	}
	return nil
}

// ValidateMinutes 额外要求每条分钟线均属于指定源年份。
func ValidateMinutes(year int, from string, rows []*KlineMinute) error {
	if err := ValidateTimeRange(from, "", protocol.TimeLayout); err != nil {
		return err
	}
	for _, k := range rows {
		if k == nil {
			return fmt.Errorf("model: nil 分钟线")
		}
		t, err := parseTime(k.Time, protocol.TimeLayout)
		if err != nil {
			return err
		}
		if t.Year() != year {
			return fmt.Errorf("model: 源时间 %s 不属于 %d 年", k.Time, year)
		}
	}
	return nil
}
