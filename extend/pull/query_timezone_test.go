package pull

import (
	"testing"
	"time"
)

func TestQueryMinuteUsesStorageTimezone(t *testing.T) {
	for _, tt := range []struct {
		name string
		at   time.Time
	}{
		{"east", time.Date(2026, 1, 1, 0, 1, 0, 0, time.FixedZone("east", 8*3600))},
		{"west", time.Date(2025, 12, 31, 23, 59, 0, 0, time.FixedZone("west", -5*3600))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := testService(t)
			s.start = tt.at
			code := Code{Market: MarketFuture, Code: "shf/cu2601"}
			if err := s.SaveMinute(code, tt.at.Year(), 0, []*KlineMinute{{Unix: tt.at.Unix(), Close: 10}}); err != nil {
				t.Fatal(err)
			}
			start, end := tt.at.Add(-time.Minute), tt.at.Add(time.Minute)
			for _, bounds := range [][2]time.Time{{start, end}, {start.UTC(), end.UTC()}, {{}, end.UTC()}, {start.UTC(), {}}} {
				rows, err := s.QueryMinute(code, bounds[0], bounds[1])
				if err != nil || len(rows) != 1 || rows[0].Unix != tt.at.Unix() {
					t.Fatal("同一时间范围因时区变化而漏记录", bounds, rows, err)
				}
			}
		})
	}
}
