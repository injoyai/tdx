package protocol

import (
	"fmt"
	"time"
)

var location = time.FixedZone("UTC+08:00", 8*60*60)

// TimeLayout 为带时分的源时间格式，不包含时区。
const TimeLayout = "2006-01-02 15:04"

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
