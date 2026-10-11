package protocol

import "testing"

func TestVWAPConvertsLotsToShares(t *testing.T) {
	tests := []struct {
		name string
		ks   Klines
		want Price
	}{
		{"empty", nil, 0},
		{"zero volume", Klines{{Volume: 0, Amount: 0}}, 0},
		{"one trade", Klines{{Volume: 1, Amount: Yuan(1000)}}, Yuan(10)},
		{"weighted", Klines{{Volume: 1, Amount: Yuan(1000)}, {Volume: 3, Amount: Yuan(6000)}}, Yuan(17.5)},
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
		{High: 10000, Low: 10000, Close: 10000},
		{High: 13000, Low: 8000, Close: 10000},  // TR=5
		{High: 11000, Low: 9000, Close: 10000},  // TR=2
		{High: 19000, Low: 12000, Close: 15000}, // TR=9 (gap from previous close)
	}
	for _, tt := range []struct {
		name string
		ks   Klines
		n    int
		want Price
	}{
		{"empty", nil, 1, 0},
		{"invalid period", ks, 0, 0},
		{"insufficient", ks[:2], 2, 0},
		{"period one", ks[:2], 1, Yuan(5)},
		{"first full window", ks[:3], 2, Yuan(3.5)},
		{"shifted window", ks, 2, Yuan(5.5)},
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
		prices []Price
		n      int
		want   int64
	}{
		{"empty", nil, 3, 0},
		{"invalid period", []Price{10, 11}, 0, 0},
		{"insufficient", []Price{10, 11}, 3, 0},
		{"rising", []Price{10, 11, 12, 13}, 3, 100},
		{"falling", []Price{13, 12, 11, 10}, 3, 0},
		{"mixed", []Price{10, 12, 11}, 2, 66},
		{"last loss leaves window", []Price{10, 9, 10, 11}, 2, 100},
		{"flat", []Price{10, 10, 10}, 2, 0},
		{"last gain leaves flat window", []Price{10, 11, 11, 11}, 2, 0},
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
		{High: 11000, Low: 9000, Close: 10000, Volume: 1, Amount: Yuan(1000)},
		{High: 13000, Low: 11000, Close: 12000, Volume: 3, Amount: Yuan(3600)},
		{High: 12000, Low: 10000, Close: 11000},
		{High: 14000, Low: 12000, Close: 13000},
		{High: 15000, Low: 13000, Close: 14000},
	}
	if got := ks.VWAP(); got != Yuan(11.5) {
		t.Fatalf("VWAP=%v, want 11.5", got)
	}
	if got := ks[:2].ATR(1); got != Yuan(3) {
		t.Fatalf("ATR(1)=%v, want 3", got)
	}
	if got := ks.ATR(2); got != Yuan(2.5) {
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
	ks := Klines{{Close: Yuan(10)}}
	for _, n := range []int{-1, 0, 2} {
		if got := ks.MA(n); got != 0 {
			t.Fatalf("MA(%d)=%v, want 0", n, got)
		}
	}
}
