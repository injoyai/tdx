package tdx

import (
	"testing"
	"time"
)

func TestHistoryTradeRejectsTypedNilCalendar(t *testing.T) {
	// 没有行情来源：空日历必须在访问数据源之前报错。
	q := &queries{}
	for _, calendar := range []TradingCalendar{nil, (*completionCalendar)(nil)} {
		if r, err := q.GetHistoryTradeBefore("sz000001", calendar, time.Now()); r != nil || err == nil || err.Error() != "交易日历不能为空" {
			t.Fatalf("accepted nil calendar: %v, %v", r, err)
		}
	}
}
