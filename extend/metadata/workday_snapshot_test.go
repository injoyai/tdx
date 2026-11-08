package metadata

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCalendarCoverageAndOwnership(t *testing.T) {
	snapshot := model.WorkdaySnapshot{Market: protocol.MarketAStock, Source: "fixture", From: "2026-01-02", Through: "2026-01-05", Dates: []string{"2026-01-02", "2026-01-05"}}
	w, err := NewWorkday(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Dates[0] = "2026-01-03"
	w.Snapshot().Dates[0] = "2026-01-04"
	for _, tc := range []struct {
		date        string
		open, known bool
	}{{"2026-01-02", true, true}, {"2026-01-03", false, true}, {"2026-01-06", false, false}} {
		at, _ := time.ParseInLocation(time.DateOnly, tc.date, protocol.Location())
		open, known := w.Status(at.UTC())
		if open != tc.open || known != tc.known {
			t.Fatal(tc, open, known)
		}
	}
	var dates []string
	w.RangeYear(2026, func(at time.Time) bool { dates = append(dates, at.Format(time.DateOnly)); return true })
	if len(dates) != 2 || dates[0] != "2026-01-02" || dates[1] != "2026-01-05" {
		t.Fatal(dates)
	}
	snapshot.Dates = []string{"2026-02-30"}
	if _, err := NewWorkday(snapshot); err == nil {
		t.Fatal("accepted malformed date")
	}
	snapshot = w.Snapshot()
	snapshot.Market = protocol.MarketUS
	if _, err := NewWorkday(snapshot); err == nil {
		t.Fatal("mainland calendar silently accepted a foreign market")
	}
}
