package sqlite

import (
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestNullableAmountAndLegacyZero(t *testing.T) {
	s := testStore(t)
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	// Existing numeric zero must keep its original interpretation on reopen.
	db, release, err := s.openDay(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO KlineDay (Time, Amount) VALUES ('2026-01-02', 0)"); err != nil {
		t.Fatal(err)
	}
	release()
	for _, row := range []*model.KlineDay{
		{Time: "2026-01-05", Amount: nil},
		{Time: "2026-01-06", Amount: testAmount(0)},
		{Time: "2026-01-07", Amount: testAmount(123.45)},
	} {
		if err := s.SaveDay(c, row.Time, []*model.KlineDay{row}); err != nil {
			t.Fatal(err)
		}
	}
	mins := []*model.KlineMinute{
		{Time: "2026-01-05 09:31", Amount: nil},
		{Time: "2026-01-05 09:32", Amount: testAmount(0)},
		{Time: "2026-01-05 09:33", Amount: testAmount(123.45)},
	}
	if err := s.SaveMinute(c, 2026, "", mins, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewPull(s.dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	days, err := s.QueryDay(c, "", "")
	if err != nil || len(days) != 4 {
		t.Fatal(days, err)
	}
	if days[0].Amount == nil || *days[0].Amount != 0 || days[1].Amount != nil || days[2].Amount == nil || *days[2].Amount != 0 || days[3].Amount == nil || *days[3].Amount != 123.45 {
		t.Fatal("day amount validity lost", days)
	}
	got, err := s.QueryMinute(c, "", "")
	if err != nil || len(got) != 3 || got[0].Amount != nil || got[1].Amount == nil || *got[1].Amount != 0 || got[2].Amount == nil || *got[2].Amount != 123.45 {
		t.Fatal("minute amount validity lost", got, err)
	}
	db, release, err = s.openDay(c)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	rows, err := db.QueryString("SELECT Time FROM KlineDay WHERE Amount IS NULL")
	if err != nil || len(rows) != 1 || rows[0]["Time"] != "2026-01-05" {
		t.Fatal("unknown amount not stored as SQL NULL", rows, err)
	}
}
