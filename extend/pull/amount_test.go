package pull

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestLegacyExtendedAmountQuery(t *testing.T) {
	s := testService(t)
	c := Code{Market: MarketFuture, Code: "cff/IF2609"}
	at := time.Date(2026, 10, 20, 15, 0, 0, 0, protocol.Location())
	for _, tc := range []struct{ file, table string }{
		{s.DayFile(c), "KlineDay"}, {s.MinuteFile(c, 2026), "KlineMinute"},
	} {
		db, err := xorms.NewSQLite(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TABLE " + tc.table + " (Unix INTEGER PRIMARY KEY, Amount REAL)"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("INSERT INTO "+tc.table+" (Unix,Amount) VALUES (?,?)", at.Unix(), 123.5); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
	days, err := s.QueryDay(c, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	minutes, err := s.QueryMinute(c, at.Add(-time.Hour), at)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || !days[0].AmountUnknown || days[0].Amount != 0 {
		t.Fatal("old day amount exposed", days)
	}
	if len(minutes) != 1 || !minutes[0].AmountUnknown || minutes[0].Amount != 0 {
		t.Fatal("old minute amount exposed", minutes)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, table string }{
		{s.DayFile(c), "KlineDay"}, {s.MinuteFile(c, 2026), "KlineMinute"},
	} {
		db, err := xorms.NewSQLite(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		var raw struct {
			Amount        float64
			AmountUnknown bool
		}
		has, err := db.Table(tc.table).Get(&raw)
		db.Close()
		if err != nil || !has || raw.Amount != 123.5 || raw.AmountUnknown {
			t.Fatalf("query changed original row: %+v %v", raw, err)
		}
	}
}

func TestAmountUnknownStorageAndAggregation(t *testing.T) {
	s := testService(t)
	c := Code{Market: MarketAStock, Code: "sz000001"}
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
