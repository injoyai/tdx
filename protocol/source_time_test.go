package protocol

import (
	"testing"
	"time"
)

func TestParseSourceTimeRequiresKnownUnambiguousClock(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"2026-03-08 02:30", "2026-11-01 01:30", "2026-01-05", "2026-01-05 9:30", "invalid"} {
		if _, err := ParseSourceTime(value, loc); err == nil {
			t.Fatalf("accepted ambiguous, missing or invalid time %q", value)
		}
	}
	if _, err := ParseSourceTime("2026-01-05 09:30", nil); err == nil {
		t.Fatal("guessed unknown timezone")
	}
	for _, tc := range []struct {
		value  string
		offset int
	}{{"2026-01-05 09:30", -5 * 3600}, {"2026-07-06 09:30", -4 * 3600}} {
		got, err := ParseSourceTime(tc.value, loc)
		if err != nil {
			t.Fatal(err)
		}
		_, offset := got.Zone()
		if offset != tc.offset || got.Format(TimeLayout) != tc.value {
			t.Fatal(got, offset)
		}
	}
}

func TestKlineTimeKeepsSourcePrecision(t *testing.T) {
	at := time.Date(2026, 1, 5, 9, 31, 0, 0, Location())
	for _, category := range []uint8{TypeKlineDay, TypeKlineDay2, TypeKlineWeek, TypeKlineMonth, TypeKlineQuarter, TypeKlineYear} {
		if got := KlineTime(at, category); got != "2026-01-05" {
			t.Fatal(category, got)
		}
	}
	if got := KlineTime(at, TypeKlineMinute); got != "2026-01-05 09:31" {
		t.Fatal(got)
	}
}
