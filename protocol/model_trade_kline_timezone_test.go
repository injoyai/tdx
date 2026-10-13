package protocol

import (
	"testing"
	"time"
)

func TestTradesKlinesTimezone(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	trades := Trades{
		{Time: time.Date(2026, 10, 12, 10, 22, 0, 0, location), Price: 10, Volume: 1},
		{Time: time.Date(2026, 10, 9, 10, 22, 0, 0, location), Price: 10, Volume: 1},
	}
	ks := trades.Klines()
	if len(ks) != 2*241 {
		t.Fatalf("got %d bars, want 482", len(ks))
	}
	for i, day := range []int{9, 12} {
		for j, k := range ks[i*241 : (i+1)*241] {
			_, offset := k.Time.Zone()
			if k.Time.Day() != day || offset != 8*60*60 {
				t.Fatalf("day %d bar %d: got %s, want original date and UTC+8", day, j, k.Time.Format(time.RFC3339))
			}
		}
		first := time.Date(2026, 10, day, 9, 30, 0, 0, location)
		last := time.Date(2026, 10, day, 15, 0, 0, 0, location)
		if !ks[i*241].Time.Equal(first) || !ks[(i+1)*241-1].Time.Equal(last) {
			t.Fatalf("day %d: incorrect session boundaries", day)
		}
		if k := ks[i*241+53]; k.Volume != 1 || k.Close != 10 {
			t.Fatalf("day %d: trade not preserved in 10:23 bar: %+v", day, k)
		}
	}
}

func TestDecodedTradesKlinesTimezone(t *testing.T) {
	for _, tt := range []struct {
		name   string
		data   []byte
		decode func([]byte, TradeCache) (*TradeResp, error)
	}{
		{"intraday", []byte{1, 0, 0x6e, 0x02, 1, 1, 1, 0, 0}, MTrade.Decode},
		{"historical", []byte{1, 0, 0, 0, 0, 0, 0x6e, 0x02, 1, 1, 0, 0}, MHistoryTrade.Decode},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.decode(tt.data, TradeCache{Date: "20261009", Code: "sh600519"})
			if err != nil {
				t.Fatal(err)
			}
			ks := resp.List.Klines()
			if len(ks) != 241 {
				t.Fatalf("got %d bars, want 241", len(ks))
			}
			want := resp.List[0].Time.Add(time.Minute).Format(time.RFC3339)
			if got := ks[53].Time.Format(time.RFC3339); got != want {
				t.Fatalf("10:23 bar: got %s, want %s", got, want)
			}
		})
	}
}
