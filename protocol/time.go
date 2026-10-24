package protocol

import (
	"fmt"
	"time"
)

var location = time.FixedZone("UTC+08:00", 8*60*60)

// TimeLayout 为带时分的源时间格式，不包含时区。
const TimeLayout = "2006-01-02 15:04"

// ParseSourceTime 将已确认来源时区的日期时分转换为真实时刻。
// 日期标签不代表时刻；拒绝缺少时区、夏令时跳时或重复时刻，避免静默猜测。
func ParseSourceTime(value string, loc *time.Location) (time.Time, error) {
	if loc == nil || loc.String() == "Local" {
		return time.Time{}, fmt.Errorf("源时间 %q 的时区未知", value)
	}
	t, err := time.ParseInLocation(TimeLayout, value, loc)
	if err != nil {
		return time.Time{}, err
	}
	if t.Format(TimeLayout) != value {
		return time.Time{}, fmt.Errorf("源时间 %q 在 %s 不存在", value, loc)
	}
	_, offset := t.Zone()
	for _, adjacent := range []time.Time{t.Add(-24 * time.Hour), t.Add(24 * time.Hour)} {
		_, other := adjacent.Zone()
		if other != offset && t.Add(time.Duration(offset-other)*time.Second).In(loc).Format(TimeLayout) == value {
			return time.Time{}, fmt.Errorf("源时间 %q 在 %s 重复，需明确 UTC 偏移", value, loc)
		}
	}
	return t, nil
}

// IsIntraday 判断 K 线周期是否携带时分。
func IsIntraday(category uint8) bool {
	switch category {
	case TypeKlineMinute, TypeKlineMinute2, TypeKline5Minute, TypeKline15Minute, TypeKline30Minute, TypeKline60Minute:
		return true
	}
	return false
}

// KlineTime 保留源时间精度；日及更长周期只有日期，不添加收盘时刻。
func KlineTime(t time.Time, category uint8) string {
	if IsIntraday(category) {
		return t.Format(TimeLayout)
	}
	return t.Format(time.DateOnly)
}

// Location 返回标准行情使用的固定东八区，不依赖运行环境的 TZ。
// 扩展行情的钟面时间仍按原值保留，不推断证券当地时区或夜盘自然日。
func Location() *time.Location { return location }

func GetHourMinute(bs [2]byte) string {
	n := Uint16(bs[:])
	h := n / 60
	m := n % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func GetTime(bs [4]byte, category uint8) time.Time {
	switch category {
	case TypeKlineMinute, TypeKlineMinute2, TypeKline5Minute, TypeKline15Minute, TypeKline30Minute, TypeKline60Minute:

		yearMonthDay := Uint16(bs[:2])
		hourMinute := Uint16(bs[2:4])
		year := int(yearMonthDay>>11 + 2004)
		month := yearMonthDay % 2048 / 100
		day := int((yearMonthDay % 2048) % 100)
		hour := int(hourMinute / 60)
		minute := int(hourMinute % 60)
		return time.Date(year, time.Month(month), day, hour, minute, 0, 0, Location())

	default:

		yearMonthDay := Uint32(bs[:4])
		year := int(yearMonthDay / 10000)
		month := int((yearMonthDay % 10000) / 100)
		day := int(yearMonthDay % 100)
		return time.Date(year, time.Month(month), day, 15, 0, 0, 0, Location())

	}
}

func minutes(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
