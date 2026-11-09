package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestUnsafeCodesCannotReadOrOverwriteAnotherSecurity(t *testing.T) {
	base := t.TempDir()
	s, err := NewPull(filepath.Join(base, "dataset"), 2)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	good := model.Code{Market: "custom", Code: "A-B"}
	day := []*model.KlineDay{{Time: "2026-01-05", Close: 10}}
	minute := []*model.KlineMinute{{Time: "2026-01-05 09:31", Close: 10}}
	if err := s.SaveDay(good, "", day); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(good, 2026, "", minute, nil); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []model.Code{
		{Market: "custom", Code: "A/B"}, {Market: "custom", Code: `A\B`},
		{Market: "../escaped", Code: "A"}, {Market: "a/../custom", Code: "A"},
		{Market: "custom", Code: "A*"}, {Market: "custom", Code: "A[1]"},
		{Market: "custom", Code: "A."}, {Market: "custom", Code: "CON"},
		{Market: "", Code: "A"}, {Market: "custom", Code: ""},
		{Market: protocol.MarketFuture, Code: "cff-IF2609"},
	} {
		t.Run(bad.Market.String()+":"+bad.Code, func(t *testing.T) {
			if err := s.SaveDay(bad, "", day); err == nil {
				t.Fatal("unsafe daily write accepted")
			}
			if err := s.SaveMinute(bad, 2026, "", minute, nil); err == nil {
				t.Fatal("unsafe minute write accepted")
			}
			if _, err := s.QueryDay(bad, "", ""); err == nil {
				t.Fatal("unsafe daily read accepted")
			}
			if _, err := s.QueryMinute(bad, "", ""); err == nil {
				t.Fatal("unsafe minute read accepted")
			}
			if _, err := s.LastDayTime(bad); err == nil {
				t.Fatal("unsafe last-day read accepted")
			}
			if _, err := s.LastMinuteTime(bad, 2026); err == nil {
				t.Fatal("unsafe last-minute read accepted")
			}
			if _, err := s.QueryMinuteCoverage(bad, 2026); err == nil {
				t.Fatal("unsafe coverage read accepted")
			}
			if _, err := s.LoadMinuteEmptyScan(bad, 2026); err == nil {
				t.Fatal("unsafe scan read accepted")
			}
			if err := s.SaveMinuteEmptyScan(bad, 2026, model.MinuteEmptyScan{}); err == nil {
				t.Fatal("unsafe scan write accepted")
			}
			if s.DayFile(bad) != "" || s.MinuteFile(bad, 2026) != "" || s.MinuteExists(bad, 2026) {
				t.Fatal("unsafe file path escaped validation")
			}
		})
	}
	got, err := s.QueryDay(good, "", "")
	if err != nil || len(got) != 1 || got[0].Close != 10 {
		t.Fatal("another security's daily data changed", got, err)
	}
	mins, err := s.QueryMinute(good, "", "")
	if err != nil || len(mins) != 1 || mins[0].Close != 10 {
		t.Fatal("another security's minute data changed", mins, err)
	}
	if _, err := os.Stat(filepath.Join(base, "escaped")); !os.IsNotExist(err) {
		t.Fatal("market path escaped dataset", err)
	}
}

func TestMinutePathsRejectWrappedYears(t *testing.T) {
	s := testStore(t)
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	for _, year := range []int{-1, 0, 10000} {
		if _, err := s.LastMinuteTime(c, year); err == nil || s.MinuteFile(c, year) != "" {
			t.Fatal("invalid year mapped to another year", year, err)
		}
		if err := s.SaveMinute(c, year, "", nil, nil); err == nil {
			t.Fatal("invalid year accepted on empty write", year)
		}
	}
}
