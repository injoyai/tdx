package pull

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func testAmount(v float64) *float64 { return &v }

func TestAmountPreservesStoredValue(t *testing.T) {
	s := testService(t)
	c := model.Code{Market: protocol.MarketFuture, Code: "cff/IF2609"}
	at := time.Date(2026, 10, 20, 15, 0, 0, 0, protocol.Location())
	if err := s.SaveDay(c, "", []*model.KlineDay{{Time: at.Format(time.DateOnly), Amount: testAmount(123)}, {Time: at.AddDate(0, 0, 1).Format(time.DateOnly), Amount: testAmount(456)}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(c, 2026, "", []*model.KlineMinute{{Time: at.Format("2006-01-02 15:04"), Amount: testAmount(123)}, {Time: at.Add(time.Minute).Format("2006-01-02 15:04"), Amount: testAmount(456)}}); err != nil {
		t.Fatal(err)
	}
	days, err := s.QueryDay(c, "", "")
	if err != nil || len(days) != 2 || (days[0].Amount == nil || *days[0].Amount != 123) || (days[1].Amount == nil || *days[1].Amount != 456) {
		t.Fatal(days, err)
	}
	minutes, err := s.QueryMinute(c, at.Format("2006-01-02 15:04"), at.Add(time.Minute).Format("2006-01-02 15:04"))
	if err != nil || len(minutes) != 2 || (minutes[0].Amount == nil || *minutes[0].Amount != 123) || (minutes[1].Amount == nil || *minutes[1].Amount != 456) {
		t.Fatal(minutes, err)
	}
}

func TestAmountStorageAndAggregation(t *testing.T) {
	s := testService(t)
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	at := time.Date(2026, 10, 19, 9, 31, 0, 0, protocol.Location())
	days := []*model.KlineDay{{Time: at.Format(time.DateOnly), Amount: testAmount(100)}, {Time: at.AddDate(0, 0, 1).Format(time.DateOnly), Amount: testAmount(0)}}
	minutes := []*model.KlineMinute{{Time: at.Format("2006-01-02 15:04"), Amount: testAmount(100)}, {Time: at.Add(time.Minute).Format("2006-01-02 15:04"), Amount: testAmount(0)}}
	if err := s.SaveDay(c, "", days); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(c, 2026, "", minutes); err != nil {
		t.Fatal(err)
	}
	gotDay, err := s.QueryDay(c, "", "")
	if err != nil {
		t.Fatal(err)
	}
	gotMin, err := s.QueryMinute(c, at.Format("2006-01-02 15:04"), at.Add(time.Minute).Format("2006-01-02 15:04"))
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDay) != 2 || (gotDay[0].Amount == nil || *gotDay[0].Amount != 100) {
		t.Fatal(gotDay)
	}
	if len(gotMin) != 2 || (gotMin[0].Amount == nil || *gotMin[0].Amount != 100) {
		t.Fatal(gotMin)
	}
	calendar, err := DayToCalendar(days, CalendarWeek)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := MinuteToSessions(minutes, 5, []TradingSession{{StartMinute: 9*60 + 30, EndMinute: 11*60 + 30}})
	if err != nil {
		t.Fatal(err)
	}
	for _, rows := range [][]*model.KlineDay{DayToPeriod(days, 2), calendar} {
		if len(rows) != 1 || (rows[0].Amount == nil || *rows[0].Amount != 100) {
			t.Fatal("daily amount sum changed", rows)
		}
	}
	for _, rows := range [][]*model.KlineMinute{MinuteToPeriod(minutes, 2), sessions} {
		if len(rows) != 1 || (rows[0].Amount == nil || *rows[0].Amount != 100) {
			t.Fatal("minute amount sum changed", rows)
		}
	}
	if (days[0].Amount == nil || *days[0].Amount != 100) || (minutes[0].Amount == nil || *minutes[0].Amount != 100) {
		t.Fatal("aggregation mutated input")
	}
}

func TestAggregationKeepsUnknownAmount(t *testing.T) {
	for _, unknownFirst := range []bool{false, true} {
		a, b := testAmount(100), (*float64)(nil)
		if unknownFirst {
			a, b = b, a
		}
		days := []*model.KlineDay{{Time: "2026-01-05", Amount: a}, {Time: "2026-01-06", Amount: b}}
		minutes := []*model.KlineMinute{{Time: "2026-01-05 09:31", Amount: a}, {Time: "2026-01-05 09:32", Amount: b}}
		calendar, err := DayToCalendar(days, CalendarWeek)
		if err != nil {
			t.Fatal(err)
		}
		sessions, err := MinuteToSessions(minutes, 5, []TradingSession{{StartMinute: 570, EndMinute: 690}})
		if err != nil {
			t.Fatal(err)
		}
		if DayToPeriod(days, 2)[0].Amount != nil || calendar[0].Amount != nil || MinuteToPeriod(minutes, 2)[0].Amount != nil || sessions[0].Amount != nil {
			t.Fatal("unknown amount became a partial sum")
		}
		known := a
		if known == nil {
			known = b
		}
		if *known != 100 {
			t.Fatal("aggregation changed input amount")
		}
	}
}
