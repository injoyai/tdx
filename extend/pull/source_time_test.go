package pull

import (
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestSourceTimeValidationAndKnownTimestamp(t *testing.T) {
	s := testService(t)
	code := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	stamp, err := (KlineMinute{Time: "2026-01-05 09:31"}).Timestamp(protocol.Location())
	if err != nil {
		t.Fatal(err)
	}
	row := &KlineMinute{Time: "2026-01-05 09:31", TradingDate: "2026-01-05", Unix: &stamp, TimeZone: protocol.Location().String(), Close: 10}
	if err := s.SaveMinute(code, 2026, "", []*KlineMinute{row}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMinute(code, "", "")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], row) {
		t.Fatal(got, err)
	}
	for _, bad := range []*KlineMinute{
		nil,
		{Time: "2025-01-05 09:31"},
		{Time: "2026-02-30 09:31"},
		{Time: "2026-01-05 9:31"},
		{Time: row.Time, Unix: &stamp},
		{Time: row.Time, Unix: &stamp, TimeZone: "UTC"},
		{Time: row.Time, TradingDate: "20260105"},
	} {
		if err := s.SaveMinute(code, 2026, "", []*KlineMinute{bad}); err == nil {
			t.Fatalf("accepted invalid source row %+v", bad)
		}
		if err := s.SaveMinuteComplete(code, 2026, row.Time, []*KlineMinute{bad}); err == nil {
			t.Fatalf("coverage accepted invalid row %+v", bad)
		}
	}
	got, err = s.QueryMinute(code, "", "")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], row) {
		t.Fatal("invalid write changed history", got, err)
	}
	if _, err := s.QueryMinute(code, "2026-01-06 00:00", "2026-01-05 23:59"); err == nil {
		t.Fatal("accepted reversed range")
	}
	if err := s.SaveDay(code, "", []*KlineDay{{Time: "2026-01-05 15:00"}}); err == nil {
		t.Fatal("daily label accepted a fabricated close time")
	}
	if _, err := s.MinuteYearComplete(code, 2026, "invalid"); err == nil {
		t.Fatal("invalid boundary interpreted as completed")
	}
	if _, err := s.MinuteYearRecentlyEmpty(code, 2026, "invalid", time.Now()); err == nil {
		t.Fatal("invalid scan boundary accepted")
	}
	if err := s.SaveMinuteEmptyScan(code, 2026, "invalid", time.Now()); err == nil {
		t.Fatal("invalid scan boundary persisted")
	}
}

func TestCalendarUsesSourceDateAndExplicitTradingDate(t *testing.T) {
	days := []*KlineDay{{Time: "2026-01-02", TradingDate: "2026-01-05", Volume: 1}, {Time: "2026-01-05", TradingDate: "2026-01-05", Volume: 2}}
	minutes := []*KlineMinute{{Time: "2026-01-02 21:01", TradingDate: "2026-01-05", Volume: 1}, {Time: "2026-01-03 00:01", TradingDate: "2026-01-05", Volume: 2}}
	old := time.Local
	defer func() { time.Local = old }()
	for _, loc := range []*time.Location{time.UTC, protocol.Location(), time.FixedZone("west", -8*3600)} {
		time.Local = loc
		week, err := DayToCalendar(days, CalendarWeek)
		if err != nil || len(week) != 1 || week[0].Volume != 3 || week[0].TradingDate != "2026-01-05" {
			t.Fatal(week, err)
		}
		night, err := MinuteToSessions(minutes, 240, []TradingSession{{1260, 150}})
		if err != nil || len(night) != 1 || night[0].Volume != 3 || night[0].TradingDate != "2026-01-05" || night[0].Time != "2026-01-03 00:01" {
			t.Fatal(night, err)
		}
	}
}

func TestSourceTimeStoredTimestampMatchesConversion(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	regular, err := protocol.ParseSourceTime("2026-01-05 09:31", loc)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, value string
		stamp       int64
	}{
		{"sub-minute seconds", "2026-01-05 09:31", regular.Unix() + 30},
		{"repeated DST clock", "2026-11-01 01:30", time.Date(2026, 11, 1, 1, 30, 0, 0, loc).Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t)
			code := Code{Market: protocol.MarketUS, Code: "AAPL"}
			original := &KlineMinute{Time: "2026-01-05 09:31", Close: 10}
			if err := s.SaveMinute(code, 2026, "", []*KlineMinute{original}); err != nil {
				t.Fatal(err)
			}
			bad := &KlineMinute{Time: tc.value, Unix: &tc.stamp, TimeZone: loc.String(), Close: 99}
			converted, convertErr := bad.Timestamp(loc)
			if convertErr == nil && converted == tc.stamp {
				t.Fatal("invalid fixture unexpectedly agrees with conversion")
			}
			if err := s.SaveMinute(code, 2026, "", []*KlineMinute{bad}); err == nil {
				t.Error("SaveMinute accepted a timestamp inconsistent with source conversion")
			}
			if err := s.SaveMinuteComplete(code, 2026, original.Time, []*KlineMinute{bad}); err == nil {
				t.Error("coverage accepted a timestamp inconsistent with source conversion")
			}
			rows, err := s.QueryMinute(code, "", "")
			if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], original) {
				t.Fatal("rejected timestamp changed stored history", rows, err)
			}
			coverage, err := s.QueryMinuteCoverage(code, 2026)
			if err != nil || coverage != nil {
				t.Fatal("rejected timestamp marked coverage complete", coverage, err)
			}
		})
	}
}
