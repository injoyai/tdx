package pull

import (
	"testing"
	"time"
)

func TestEmptyMinuteScanExpiryScopeAndRange(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sz000001"}
	now := time.Date(2026, 9, 4, 16, 0, 0, 0, time.Local)
	from := time.Date(2024, 3, 1, 0, 0, 0, 0, time.Local).Unix()
	if err := s.SaveMinEmptyScan(code, 2024, from, now); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		code Code
		year int
		from int64
		now  time.Time
		want bool
	}{
		{"recent", code, 2024, from, now.AddDate(0, 0, 6), true},
		{"expired", code, 2024, from, now.AddDate(0, 0, 7), false},
		{"clock rollback", code, 2024, from, now.Add(-time.Second), false},
		{"earlier start", code, 2024, from - 1, now, false},
		{"later start", code, 2024, from + 1, now, true},
		{"other year", code, 2025, from, now, false},
		{"other code", Code{Market: code.Market, Code: "sz000002"}, 2024, from, now, false},
		{"other market", Code{Market: MarketIndex, Code: code.Code}, 2024, from, now, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := s.MinYearRecentlyEmpty(tt.code, tt.year, tt.from, tt.now)
			if err != nil || got != tt.want {
				t.Fatal(got, err)
			}
		})
	}
	if s.MinExists(code, 2024) {
		t.Fatal("空历史扫描创建了行情库")
	}
	if done, err := s.MinYearComplete(code, 2024, from); err != nil || done {
		t.Fatal("空历史缓存被当成完成标记", done, err)
	}
}
