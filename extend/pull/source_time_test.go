package pull

import (
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestSourceClockStoredWithoutTimezoneConversion(t *testing.T) {
	s := testService(t)
	code := model.Code{Market: protocol.MarketUS, Code: "AAPL"}
	// 来源钟面值只需是合法日期时分，不应用纽约夏令时规则拒绝或改写它。
	want := []*model.KlineMinute{
		{Time: "2026-03-08 02:30", Close: 10},
		{Time: "2026-11-01 01:30", Close: 11},
	}
	if err := s.SaveMinuteComplete(code, 2026, "", want); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMinute(code, "", "")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("source clocks changed", got, err)
	}
}

func TestSourceTimeValidation(t *testing.T) {
	s := testService(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	row := &model.KlineMinute{Time: "2026-01-05 09:31", Close: 10}
	if err := s.SaveMinute(code, 2026, "", []*model.KlineMinute{row}); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMinute(code, "", "")
	if err != nil || len(got) != 1 || !reflect.DeepEqual(got[0], row) {
		t.Fatal(got, err)
	}
	for _, bad := range []*model.KlineMinute{
		nil,
		{Time: "2025-01-05 09:31"},
		{Time: "2026-02-30 09:31"},
		{Time: "2026-01-05 9:31"},
	} {
		if err := s.SaveMinute(code, 2026, "", []*model.KlineMinute{bad}); err == nil {
			t.Fatalf("accepted invalid source row %+v", bad)
		}
		if err := s.SaveMinuteComplete(code, 2026, row.Time, []*model.KlineMinute{bad}); err == nil {
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
	if err := s.SaveDay(code, "", []*model.KlineDay{{Time: "2026-01-05 15:00"}}); err == nil {
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

func TestCalendarUsesSourceDate(t *testing.T) {
	days := []*model.KlineDay{{Time: "2026-01-02", Volume: 1}, {Time: "2026-01-05", Volume: 2}}
	minutes := []*model.KlineMinute{{Time: "2026-01-02 21:01", Volume: 1}, {Time: "2026-01-03 00:01", Volume: 2}}
	old := time.Local
	defer func() { time.Local = old }()
	for _, loc := range []*time.Location{time.UTC, protocol.Location(), time.FixedZone("west", -8*3600)} {
		time.Local = loc
		week, err := DayToCalendar(days, CalendarWeek)
		if err != nil || len(week) != 2 || week[0].Volume != 1 || week[1].Volume != 2 {
			t.Fatal(week, err)
		}
		night, err := MinuteToSessions(minutes, 240, []TradingSession{{1260, 150}})
		if err != nil || len(night) != 1 || night[0].Volume != 3 || night[0].Time != "2026-01-03 00:01" {
			t.Fatal(night, err)
		}
	}
}
