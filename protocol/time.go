package protocol

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol/market"
)

var location = time.FixedZone("UTC+08:00", 8*60*60)

// TimeLayout 为带时分的源时间格式，不包含时区。
const TimeLayout = "2006-01-02 15:04"

// IsIntraday 判断 K 线周期是否携带时分。
func IsIntraday(category uint8) bool { return market.IsIntraday(category) }

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

// GetTime 解析协议时间。无效编码返回零值。
func GetTime(bs [4]byte, category uint8) time.Time {
	t, _ := decodeTime(bs, category)
	return t
}

func decodeTime(bs [4]byte, category uint8) (time.Time, error) {
	if IsIntraday(category) {
		date := int(Uint16(bs[:2]))
		return minuteTime(date, int(Uint16(bs[2:])))
	}
	date := int(Uint32(bs[:]))
	return checkedTime(date/10000, date/100%100, date%100, 15*60)
}

func minuteTime(date, minute int) (time.Time, error) {
	monthDay := date % 2048
	return checkedTime(date/2048+2004, monthDay/100, monthDay%100, minute)
}

func checkedTime(year, month, day, minute int) (time.Time, error) {
	t := time.Date(year, time.Month(month), day, minute/60, minute%60, 0, 0, Location())
	if year < 1 || year > 9999 || minute >= 24*60 ||
		t.Year() != year || int(t.Month()) != month || t.Day() != day {
		return time.Time{}, fmt.Errorf("无效协议时间 %04d-%02d-%02d %02d:%02d", year, month, day, minute/60, minute%60)
	}
	return t, nil
}

func minutes(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}
