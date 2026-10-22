package tdx

import "time"

// IntegerDay 返回输入时区的当日零点。
func IntegerDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
