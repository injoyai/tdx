package protocol

import (
	"testing"
	"time"
)

func TestTradesKeepTradingDayAndSession(t *testing.T) {
	previous := time.Local
	time.Local = time.UTC
	defer func() { time.Local = previous }()
	at := time.Date(2026, 10, 20, 9, 29, 0, 0, Location())
	trades := Trades{
		{Time: at, Price: 10000, Volume: 100},
		{Time: at.Add(time.Minute), Price: 11000, Volume: 1},
		{Time: time.Date(2026, 10, 20, 11, 30, 0, 0, Location()), Price: 12000, Volume: 2},
		{Time: time.Date(2026, 10, 20, 13, 0, 0, 0, Location()), Price: 13000, Volume: 3},
		{Time: at.AddDate(0, 0, 1), Price: 14000, Volume: 4},
	}
	ks := trades.Klines()
	if len(ks) != 482 {
		t.Fatal("two days must each have 241 rows", len(ks))
	}
	for _, tc := range []struct {
		index  int
		clock  string
		volume int64
	}{
		{0, "2026-10-20 09:30", 100}, {1, "2026-10-20 09:31", 1},
		{120, "2026-10-20 11:30", 2}, {121, "2026-10-20 13:01", 3},
		{240, "2026-10-20 15:00", 0}, {241, "2026-10-21 09:30", 4},
	} {
		k := ks[tc.index]
		_, offset := k.Time.Zone()
		if k.Time.Format("2006-01-02 15:04") != tc.clock || k.Volume != tc.volume || offset != 8*3600 {
			t.Fatalf("row %d: %s volume=%d, want %s volume=%d east-eight", tc.index, k.Time, k.Volume, tc.clock, tc.volume)
		}
	}
	if !trades[0].Time.Equal(at) {
		t.Fatal("input trade mutated")
	}
}

func TestTradeDecodeTimezone(t *testing.T) {
	for _, historical := range []bool{false, true} {
		bs := append(Bytes(uint16(9*60+31)), 1, 2, 0, 0)
		if !historical {
			bs = append(bs, 0)
		}
		r, err := decodeTrades(bs, 1, TradeCache{Date: "20261020", Code: "sz000001"}, historical)
		want := time.Date(2026, 10, 20, 9, 31, 0, 0, Location())
		if err != nil || len(r.List) != 1 || !r.List[0].Time.Equal(want) {
			t.Fatalf("historical=%v: %+v %v", historical, r, err)
		}
	}
}
