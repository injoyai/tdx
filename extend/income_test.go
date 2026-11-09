package extend

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestDoIncomesBounds(t *testing.T) {
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	ks := protocol.Klines{{Time: day, Close: 1}, {Time: day.AddDate(0, 0, 1), Close: 2}}
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
			got, err := DoIncomes(tc.ks, tc.start, tc.days...)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tc.want {
				t.Fatalf("got %d incomes, want %d", len(got), tc.want)
			}
			if tc.name == "normal" {
				rate, available := got[0].RiseRate()
				if got[0].Source.Close != 1 || got[0].Current.Close != 2 || !available || rate != 1 {
					t.Fatal(got[0])
				}
			}
		})
	}
}

func TestDoIncomesRejectsInvalidRecords(t *testing.T) {
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	valid := &protocol.Kline{Time: day, Close: 10}
	for _, tc := range []struct {
		name string
		ks   protocol.Klines
	}{
		{"nil first", protocol.Klines{nil, valid}},
		{"nil target", protocol.Klines{valid, nil}},
		{"nil unrequested", protocol.Klines{valid, {Time: day.AddDate(0, 0, 1), Close: 11}, nil}},
		{"NaN close", protocol.Klines{valid, {Time: day.AddDate(0, 0, 1), Close: math.NaN()}}},
		{"positive infinite close", protocol.Klines{{Time: day, Close: math.Inf(1)}}},
		{"negative infinite close", protocol.Klines{{Time: day, Close: math.Inf(-1)}}},
		{"descending time", protocol.Klines{{Time: day.AddDate(0, 0, 1), Close: 11}, valid}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DoIncomes(tc.ks, day, 1)
			if err == nil || got != nil {
				t.Fatalf("invalid records returned partial results: %v, %v", got, err)
			}
		})
	}
}

func TestDoIncomesCopiesBarsAndPreservesOffsets(t *testing.T) {
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	source := protocol.Kline{Time: day, Last: 9, Open: 9.5, High: 11, Low: 9, Close: 10, Order: 12, Volume: 100, Amount: 1000, UpCount: 3, DownCount: 4}
	current := protocol.Kline{Time: day.AddDate(0, 0, 4), Last: 10, Open: 10.5, High: 12, Low: 10, Close: 11, Order: 34, Volume: 200, Amount: 2200, UpCount: 5, DownCount: 6}
	ks := protocol.Klines{&source, &current}
	got, err := DoIncomes(ks, day, 1, 0, -1, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Offset != 1 || got[1].Offset != 0 || got[2].Offset != 1 {
		t.Fatal("offset order, duplicates or bounds changed", got)
	}
	if !reflect.DeepEqual(got[0].Source, source) || !reflect.DeepEqual(got[0].Current, current) || !got[0].Time.Equal(current.Time) {
		t.Fatal("K-line fields were lost", got[0])
	}
	source.Volume = 999
	got[0].Current.Amount = 888
	if got[0].Source.Volume != 100 || current.Amount != 2200 || got[2].Current.Amount != 2200 {
		t.Fatal("results share mutable K-line records")
	}
}

func TestIncomeRiseRateAvailability(t *testing.T) {
	for _, tc := range []struct {
		name            string
		source, current float64
		want            float64
		available       bool
	}{
		{"gain", 10, 11, 0.1, true},
		{"loss", 10, 9, -0.1, true},
		{"zero return", 10, 10, 0, true},
		{"zero base and target", 0, 0, 0, false},
		{"zero base", 0, 1, 0, false},
		{"NaN base", math.NaN(), 1, 0, false},
		{"NaN target", 1, math.NaN(), 0, false},
		{"infinite base", math.Inf(1), 1, 0, false},
		{"infinite target", 1, math.Inf(-1), 0, false},
		{"division overflow", math.SmallestNonzeroFloat64, 1, 0, false},
		{"difference overflow", math.MaxFloat64, -math.MaxFloat64, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := &Income{Source: protocol.Kline{Close: tc.source}, Current: protocol.Kline{Close: tc.current}}
			rate, available := i.RiseRate()
			if available != tc.available || rate != tc.want {
				t.Fatalf("rate=%v available=%v, want %v %v", rate, available, tc.want, tc.available)
			}
			text := i.String()
			if strings.Contains(text, "NaN") || strings.Contains(text, "Inf") || (!available && !strings.Contains(text, "不可计算")) {
				t.Fatal("invalid rate was formatted as a percentage", text)
			}
			if tc.name == "zero return" && !strings.Contains(text, "0.0%") {
				t.Fatal("valid zero return lost", text)
			}
		})
	}
	i := &Income{Source: protocol.Kline{Close: 1e-307}, Current: protocol.Kline{Close: 1}}
	if rate, available := i.RiseRate(); !available || math.IsInf(rate, 0) || !strings.Contains(i.String(), "超出显示范围") {
		t.Fatal("percentage display overflow was not distinguished from the valid rate", i)
	}
	var missing *Income
	if rate, available := missing.RiseRate(); available || rate != 0 || missing.String() != "<nil>" {
		t.Fatal("nil income reported as available")
	}
}

func TestDoIncomesZeroBaseKeepsPriceDifference(t *testing.T) {
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	got, err := DoIncomes(protocol.Klines{{Time: day}, {Time: day.AddDate(0, 0, 1), Close: 1}}, day, 1)
	if err != nil || len(got) != 1 || got[0].Rise() != 1 {
		t.Fatal("zero base lost a calculable price difference", got, err)
	}
	if rate, available := got[0].RiseRate(); available || rate != 0 {
		t.Fatal("zero-base rate was reported as available")
	}
}
