package pull

import (
	"github.com/injoyai/tdx/v2/protocol"
	"testing"
	"time"
)

func TestQueryMinuteUsesSourceCalendar(t *testing.T) {
	s := testService(t)
	code := Code{Market: protocol.MarketUS, Code: "AAPL"}
	for _, value := range []string{"2025-12-31 23:59", "2026-01-01 00:01"} {
		year := 2025
		if value[:4] == "2026" {
			year = 2026
		}
		if err := s.SaveMinute(code, year, "", []*KlineMinute{{Time: value, Close: 10}}); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Local
	defer func() { time.Local = old }()
	for _, loc := range []*time.Location{time.UTC, protocol.Location(), time.FixedZone("west", -5*3600)} {
		time.Local = loc
		for _, bounds := range [][2]string{{"", ""}, {"2025-12-31 23:59", "2026-01-01 00:01"}, {"", "2026-01-01 00:01"}, {"2025-12-31 23:59", ""}} {
			rows, err := s.QueryMinute(code, bounds[0], bounds[1])
			if err != nil || len(rows) != 2 || rows[0].Time != "2025-12-31 23:59" || rows[1].Time != "2026-01-01 00:01" {
				t.Fatalf("host=%s rows=%v err=%v", loc, rows, err)
			}
		}
	}
}
