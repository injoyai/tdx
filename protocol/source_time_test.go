package protocol

import (
	"testing"
	"time"
)

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
