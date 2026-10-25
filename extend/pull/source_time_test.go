package pull

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestSourceClockStoredWithoutTimezoneConversion(t *testing.T) {
	s := testService(t)
	code := Code{Market: protocol.MarketUS, Code: "AAPL"}
	// 来源钟面值只需是合法日期时分，不应用纽约夏令时规则拒绝或改写它。
	want := []*KlineMinute{
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

func TestMinuteSchemaWithoutTimestampColumns(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "minute.db")
	db, err := openDB(filename, new(KlineMinute))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	columns, err := db.QueryString("PRAGMA table_info('KlineMinute')")
	if err != nil || len(columns) != minCols {
		t.Fatal("minute column count", columns, err)
	}
	for _, col := range columns {
		if col["name"] == "Unix" || col["name"] == "TimeZone" || col["name"] == "TradingDate" || col["name"] == "AmountUnknown" {
			t.Fatal("timestamp column created", col)
		}
	}
	// 模拟上一版源时间主键库；已有额外列不应触发删表或损坏源时间记录。
	for _, sql := range []string{
		"ALTER TABLE KlineMinute ADD COLUMN Unix BIGINT NULL",
		"ALTER TABLE KlineMinute ADD COLUMN TradingDate TEXT NULL",
		"ALTER TABLE KlineMinute ADD COLUMN AmountUnknown INTEGER DEFAULT 0",
		"ALTER TABLE KlineMinute ADD COLUMN TimeZone TEXT NULL",
		"INSERT INTO KlineMinute (Time, Close, Unix, TimeZone) VALUES ('2026-01-05 09:31', 10, 1234567890, 'UTC+08:00')",
	} {
		if _, err := db.Exec(sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDB(filename, new(KlineMinute))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := upsertMin(db, "2026-01-05 09:32", []*KlineMinute{{Time: "2026-01-05 09:32", Close: 11}}); err != nil {
		t.Fatal(err)
	}
	rows, err := queryMin(db, "", "")
	if err != nil || len(rows) != 2 || rows[0].Close != 10 || rows[1].Close != 11 {
		t.Fatal("legacy extra columns prevented read/write", rows, err)
	}
	legacy, err := db.QueryString("SELECT Unix, TimeZone FROM KlineMinute WHERE Time='2026-01-05 09:31'")
	if err != nil || len(legacy) != 1 || legacy[0]["Unix"] != "1234567890" || legacy[0]["TimeZone"] != "UTC+08:00" {
		t.Fatal("legacy data changed", legacy, err)
	}
}

func TestSourceTimeValidation(t *testing.T) {
	s := testService(t)
	code := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	row := &KlineMinute{Time: "2026-01-05 09:31", Close: 10}
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

func TestCalendarUsesSourceDate(t *testing.T) {
	days := []*KlineDay{{Time: "2026-01-02", Volume: 1}, {Time: "2026-01-05", Volume: 2}}
	minutes := []*KlineMinute{{Time: "2026-01-02 21:01", Volume: 1}, {Time: "2026-01-03 00:01", Volume: 2}}
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
