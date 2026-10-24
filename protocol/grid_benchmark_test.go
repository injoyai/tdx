package protocol

import (
	"testing"
	"time"
)

func BenchmarkProtocolGrid(b *testing.B) {
	var ks Klines
	var trades Trades
	for day := 2; day < 12; day++ {
		for minute := 570; minute <= 900; minute++ {
			if minute > 690 && minute < 781 {
				continue
			}
			at := time.Date(2025, 1, day, minute/60, minute%60, 0, 0, time.Local)
			ks = append(ks, &Kline{Time: at, Open: 10000, High: 11000, Low: 9000, Close: 10500, Volume: 1})
			trades = append(trades, &Trade{Time: at, Price: 10500, Volume: 1, Number: 1})
		}
	}
	b.Run("Merge241TenDays", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkResult = ks.Merge241(15)
		}
	})
	b.Run("TradesTenDays", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkResult = trades.Klines()
		}
	})
}
