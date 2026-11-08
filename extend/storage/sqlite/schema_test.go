package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestMinuteSchemaWithoutTimestampColumns(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "minute.db")
	db, err := openDB(filename, new(model.KlineMinute))
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
	db, err = openDB(filename, new(model.KlineMinute))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := upsertMin(db, "2026-01-05 09:32", []*model.KlineMinute{{Time: "2026-01-05 09:32", Close: 11}}); err != nil {
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
