package pull

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestAmountPreservesStoredValue(t *testing.T) {
	s := testService(t)
	c := Code{Market: protocol.MarketFuture, Code: "cff/IF2609"}
	at := time.Date(2026, 10, 20, 15, 0, 0, 0, protocol.Location())
	if err := s.SaveDay(c, "", []*KlineDay{{Time: at.Format(time.DateOnly), Amount: 123}, {Time: at.AddDate(0, 0, 1).Format(time.DateOnly), Amount: 456}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(c, 2026, "", []*KlineMinute{{Time: at.Format("2006-01-02 15:04"), Amount: 123}, {Time: at.Add(time.Minute).Format("2006-01-02 15:04"), Amount: 456}}); err != nil {
		t.Fatal(err)
	}
	days, err := s.QueryDay(c, "", "")
	if err != nil || len(days) != 2 || days[0].Amount != 123 || days[1].Amount != 456 {
		t.Fatal(days, err)
	}
	minutes, err := s.QueryMinute(c, at.Format("2006-01-02 15:04"), at.Add(time.Minute).Format("2006-01-02 15:04"))
	if err != nil || len(minutes) != 2 || minutes[0].Amount != 123 || minutes[1].Amount != 456 {
		t.Fatal(minutes, err)
	}
}

func TestAmountStorageAndAggregation(t *testing.T) {
	s := testService(t)
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	at := time.Date(2026, 10, 19, 9, 31, 0, 0, protocol.Location())
	days := []*KlineDay{{Time: at.Format(time.DateOnly), Amount: 100}, {Time: at.AddDate(0, 0, 1).Format(time.DateOnly)}}
	minutes := []*KlineMinute{{Time: at.Format("2006-01-02 15:04"), Amount: 100}, {Time: at.Add(time.Minute).Format("2006-01-02 15:04")}}
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
	if len(gotDay) != 2 || gotDay[0].Amount != 100 {
		t.Fatal(gotDay)
	}
	if len(gotMin) != 2 || gotMin[0].Amount != 100 {
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
	for _, rows := range [][]*KlineDay{DayToPeriod(days, 2), calendar} {
		if len(rows) != 1 || rows[0].Amount != 100 {
			t.Fatal("daily amount sum changed", rows)
		}
	}
	for _, rows := range [][]*KlineMinute{MinuteToPeriod(minutes, 2), sessions} {
		if len(rows) != 1 || rows[0].Amount != 100 {
			t.Fatal("minute amount sum changed", rows)
		}
	}
	if days[0].Amount != 100 || minutes[0].Amount != 100 {
		t.Fatal("aggregation mutated input")
	}
}
