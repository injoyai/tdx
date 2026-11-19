package protocol

import (
	"testing"
	"time"
)

func TestUnknownMarketInfo(t *testing.T) {
	for _, market := range []MarketID{"", "custom"} {
		if info, known := market.Info(); known || info.Prefix != "" || len(info.Routes) != 0 {
			t.Fatalf("%q has unexpected market metadata: %+v", market, info)
		}
	}
}

func TestMarketTimeZones(t *testing.T) {
	for _, tc := range []struct {
		market       MarketID
		zone         string
		closeTime    string
		winterOffset int
		summerOffset int
	}{
		{MarketSH, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketSZ, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketBJ, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketCFF, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketQHZ, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketHK, "Asia/Hong_Kong", "16:10", 8 * 3600, 8 * 3600},
		{MarketHKIndex, "Asia/Hong_Kong", "16:10", 8 * 3600, 8 * 3600},
		{MarketUS, "America/New_York", "20:00", -5 * 3600, -4 * 3600},
	} {
		t.Run(tc.market.String(), func(t *testing.T) {
			info, known := tc.market.Info()
			if !known || info.TimeZone != tc.zone || info.CloseTime != tc.closeTime {
				t.Fatalf("unexpected market time zone: %+v, known=%v", info, known)
			}
			loc, err := time.LoadLocation(info.TimeZone)
			if err != nil {
				t.Fatal(err)
			}
			for _, date := range []struct {
				month  time.Month
				offset int
			}{{time.January, tc.winterOffset}, {time.July, tc.summerOffset}} {
				at := time.Date(2026, date.month, 15, 12, 0, 0, 0, loc)
				if _, offset := at.Zone(); offset != date.offset {
					t.Fatalf("%s: UTC offset=%d, want %d", at, offset, date.offset)
				}
			}
		})
	}
}
