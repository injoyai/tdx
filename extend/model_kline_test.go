package extend

import (
	"testing"

	"github.com/injoyai/tdx/protocol"
)

func TestKlineIndicators(t *testing.T) {
	ks := Klines{
		{Kline: &protocol.Kline{High: 11000, Low: 9000, Close: 10000, Volume: 1, Amount: protocol.Yuan(1000)}},
		{Kline: &protocol.Kline{High: 13000, Low: 11000, Close: 12000, Volume: 3, Amount: protocol.Yuan(3600)}},
		{Kline: &protocol.Kline{High: 12000, Low: 10000, Close: 11000}},
		{Kline: &protocol.Kline{High: 14000, Low: 12000, Close: 13000}},
		{Kline: &protocol.Kline{High: 15000, Low: 13000, Close: 14000}},
	}
	if got := ks.VWAP(); got != protocol.Yuan(11.5) {
		t.Fatalf("VWAP=%v, want 11.5", got)
	}
	if got := ks[:2].ATR(1); got != protocol.Yuan(3) {
		t.Fatalf("ATR(1)=%v, want 3", got)
	}
	if got := ks.ATR(2); got != protocol.Yuan(2.5) {
		t.Fatalf("ATR(2)=%v, want 2.5", got)
	}
	if got := ks[:3].RSI(2); got != 66 {
		t.Fatalf("mixed RSI=%d, want 66", got)
	}
	if got := ks.RSI(2); got != 100 {
		t.Fatalf("rising window RSI=%d, want 100", got)
	}
}
