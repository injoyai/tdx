package pull

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestAmountUsesStoredStatus(t *testing.T) {
	s := testService(t)
	c := Code{Market: protocol.MarketFuture, Code: "cff/IF2609"}
	at := time.Date(2026, 10, 20, 15, 0, 0, 0, protocol.Location())
	if err := s.SaveDay(c, 0, []*KlineDay{{Unix: at.Unix(), Amount: 123}, {Unix: at.Unix() + 1, Amount: 456, AmountUnknown: true}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(c, 2026, 0, []*KlineMinute{{Unix: at.Unix(), Amount: 123}, {Unix: at.Unix() + 1, Amount: 456, AmountUnknown: true}}); err != nil {
		t.Fatal(err)
	}
	days, err := s.QueryDay(c, time.Time{}, time.Time{})
	if err != nil || len(days) != 2 || days[0].AmountUnknown || days[0].Amount != 123 || !days[1].AmountUnknown || days[1].Amount != 0 {
		t.Fatal(days, err)
	}
	minutes, err := s.QueryMinute(c, at, at.Add(time.Minute))
	if err != nil || len(minutes) != 2 || minutes[0].AmountUnknown || minutes[0].Amount != 123 || !minutes[1].AmountUnknown || minutes[1].Amount != 0 {
		t.Fatal(minutes, err)
	}
}

func TestAmountUnknownStorageAndAggregation(t *testing.T) {
	s := testService(t)
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	at := time.Date(2026, 10, 19, 9, 31, 0, 0, protocol.Location())
	days := []*KlineDay{{Unix: at.Unix(), Amount: 100}, {Unix: at.AddDate(0, 0, 1).Unix(), AmountUnknown: true}}
	minutes := []*KlineMinute{{Unix: at.Unix(), Amount: 100}, {Unix: at.Add(time.Minute).Unix(), AmountUnknown: true}}
	if err := s.SaveDay(c, 0, days); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(c, 2026, 0, minutes); err != nil {
		t.Fatal(err)
	}
	gotDay, err := s.QueryDay(c, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	gotMin, err := s.QueryMinute(c, at, at.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(gotDay) != 2 || gotDay[0].AmountUnknown || !gotDay[1].AmountUnknown || gotDay[0].Amount != 100 {
		t.Fatal(gotDay)
	}
	if len(gotMin) != 2 || gotMin[0].AmountUnknown || !gotMin[1].AmountUnknown || gotMin[0].Amount != 100 {
		t.Fatal(gotMin)
	}
	calendar, err := DayToCalendar(days, CalendarWeek, protocol.Location())
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := MinuteToSessions(minutes, 5, protocol.Location(), []TradingSession{{StartMinute: 9*60 + 30, EndMinute: 11*60 + 30}})
	if err != nil {
		t.Fatal(err)
	}
	for _, rows := range [][]*KlineDay{DayToPeriod(days, 2), calendar} {
		if len(rows) != 1 || rows[0].Amount != 0 || !rows[0].AmountUnknown {
			t.Fatal("partial daily amount exposed", rows)
		}
	}
	for _, rows := range [][]*KlineMinute{MinuteToPeriod(minutes, 2), sessions} {
		if len(rows) != 1 || rows[0].Amount != 0 || !rows[0].AmountUnknown {
			t.Fatal("partial minute amount exposed", rows)
		}
	}
	if days[0].Amount != 100 || minutes[0].Amount != 100 {
		t.Fatal("aggregation mutated input")
	}
}
