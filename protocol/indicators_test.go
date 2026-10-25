package protocol

import (
	"math"
	"testing"
)

func TestVWAPConvertsLotsToShares(t *testing.T) {
	tests := []struct {
		name string
		ks   Klines
		want float64
	}{
		{"empty", nil, 0},
		{"zero volume", Klines{{Volume: 0, Amount: 0}}, 0},
		{"one trade", Klines{{Volume: 1, Amount: 1000}}, 10},
		{"weighted", Klines{{Volume: 1, Amount: 1000}, {Volume: 3, Amount: 6000}}, 17.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ks.VWAP(); got != tt.want {
				t.Fatalf("VWAP=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestATRRollingWindow(t *testing.T) {
	ks := Klines{
		{High: 10, Low: 10, Close: 10},
		{High: 13, Low: 8, Close: 10},  // TR=5
		{High: 11, Low: 9, Close: 10},  // TR=2
		{High: 19, Low: 12, Close: 15}, // TR=9 (gap from previous close)
	}
	for _, tt := range []struct {
		name string
		ks   Klines
		n    int
		want float64
	}{
		{"empty", nil, 1, 0},
		{"invalid period", ks, 0, 0},
		{"insufficient", ks[:2], 2, 0},
		{"period one", ks[:2], 1, 5},
		{"first full window", ks[:3], 2, 3.5},
		{"shifted window", ks, 2, 5.5},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.ks.ATR(tt.n); got != tt.want {
				t.Fatalf("ATR=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestRSINoLossAndWindowChanges(t *testing.T) {
	for _, tt := range []struct {
		name   string
		prices []float64
		n      int
		want   int64
	}{
		{"empty", nil, 3, 0},
		{"invalid period", []float64{10, 11}, 0, 0},
		{"insufficient", []float64{10, 11}, 3, 0},
		{"rising", []float64{10, 11, 12, 13}, 3, 100},
		{"falling", []float64{13, 12, 11, 10}, 3, 0},
		{"mixed", []float64{10, 12, 11}, 2, 66},
		{"last loss leaves window", []float64{10, 9, 10, 11}, 2, 100},
		{"flat", []float64{10, 10, 10}, 2, 0},
		{"last gain leaves flat window", []float64{10, 11, 11, 11}, 2, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var ks Klines
			for _, price := range tt.prices {
				ks = append(ks, &Kline{Close: price})
			}
			if got := ks.RSI(tt.n); got != tt.want {
				t.Fatalf("RSI=%d, want %d", got, tt.want)
			}
		})
	}
}

func TestKlineIndicators(t *testing.T) {
	ks := Klines{
		{High: 11, Low: 9, Close: 10, Volume: 1, Amount: 1000},
		{High: 13, Low: 11, Close: 12, Volume: 3, Amount: 3600},
		{High: 12, Low: 10, Close: 11},
		{High: 14, Low: 12, Close: 13},
		{High: 15, Low: 13, Close: 14},
	}
	if got := ks.VWAP(); got != 11.5 {
		t.Fatalf("VWAP=%v, want 11.5", got)
	}
	if got := ks[:2].ATR(1); got != 3 {
		t.Fatalf("ATR(1)=%v, want 3", got)
	}
	if got := ks.ATR(2); got != 2.5 {
		t.Fatalf("ATR(2)=%v, want 2.5", got)
	}
	if got := ks[:3].RSI(2); got != 66 {
		t.Fatalf("mixed RSI=%d, want 66", got)
	}
	if got := ks.RSI(2); got != 100 {
		t.Fatalf("rising window RSI=%d, want 100", got)
	}
}

func TestMAInvalidPeriod(t *testing.T) {
	ks := Klines{{Close: 10}}
	for _, n := range []int{-1, 0, 2} {
		if got := ks.MA(n); got != 0 {
			t.Fatalf("MA(%d)=%v, want 0", n, got)
		}
	}
}

func TestIndicatorsPreserveFractionalPrices(t *testing.T) {
	ks := Klines{
		{High: 1.0002, Low: 1.0000, Close: 1.0001, Volume: 1, Amount: 100.01},
		{High: 1.0005, Low: 1.0003, Close: 1.0004, Volume: 2, Amount: 200.08},
	}
	for _, tc := range []struct {
		name      string
		got, want float64
	}{
		{"MA", ks.MA(2), 1.00025},
		{"EMA", ks.EMA(2), 1.0003},
		{"ATR", ks.ATR(1), 0.0004},
		{"VWAP", ks.VWAP(), 1.0003},
	} {
		if math.Abs(tc.got-tc.want) > 1e-12 {
			t.Errorf("%s=%g, want %g", tc.name, tc.got, tc.want)
		}
	}
	upper, mid, lower := ks.BOLL(2)
	if math.Abs(mid-1.00025) > 1e-12 || math.Abs(upper-1.00055) > 1e-12 || math.Abs(lower-0.99995) > 1e-12 {
		t.Fatal("fractional BOLL", upper, mid, lower)
	}
	dif, dea, hist := ks.MACD()
	wantDIF := 0.0003 * (2.0/13 - 2.0/27)
	if math.Abs(dif-wantDIF) > 1e-12 || math.Abs(dea-wantDIF*0.2) > 1e-12 || math.Abs(hist-wantDIF*1.6) > 1e-12 {
		t.Fatal("fractional MACD", dif, dea, hist)
	}
	if got := ks.RSI(1); got != 100 {
		t.Fatal("fractional RSI", got)
	}
	if got := (Klines{{Close: -1.0001}, {Close: -1.0004}}).MA(2); math.Abs(got-(-1.00025)) > 1e-12 {
		t.Fatal("negative MA", got)
	}
}
