package extend

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestDoIncomesBounds(t *testing.T) {
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := protocol.Klines{{Time: day, Close: 1000}, {Time: day.AddDate(0, 0, 1), Close: 2000}}
	for _, tc := range []struct {
		name  string
		ks    protocol.Klines
		start time.Time
		days  []int
		want  int
	}{
		{"empty", nil, day, []int{0}, 0},
		{"past end", ks, day.AddDate(0, 1, 0), []int{0, 1}, 0},
		{"invalid offsets", ks, day, []int{-1, 2}, 0},
		{"normal", ks, day, []int{1}, 1},
		{"matching date", ks, day.AddDate(0, 0, 1), []int{0, 1}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DoIncomes(tc.ks, tc.start, tc.days...)
			if len(got) != tc.want {
				t.Fatalf("got %d incomes, want %d", len(got), tc.want)
			}
			if tc.name == "normal" && (got[0].Source.Close != 1000 || got[0].Current.Close != 2000 || got[0].RiseRate() != 1) {
				t.Fatal(got[0])
			}
		})
	}
}
