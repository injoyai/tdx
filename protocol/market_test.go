package protocol

import (
	"testing"
	"time"
)

func TestUnknownMarketDefaults(t *testing.T) {
	for _, market := range []Market{"", "custom"} {
		info, known := market.Info()
		if known {
			t.Fatalf("%q should not be marked as a built-in market", market)
		}
		if info.Protocol != ProtocolStandard || info.Kind != KindStock ||
			info.Day.Category != TypeKlineDay || info.Minute.Category != TypeKlineMinute {
			t.Fatalf("%q has incomplete default configuration: %+v", market, info)
		}
		if info.Day.VolumeScale != 100 || info.Minute.VolumeScale != 100 {
			t.Fatalf("%q should default all volume scales to 100: %+v", market, info)
		}
	}
}

func TestMarketTimeZones(t *testing.T) {
	for _, tc := range []struct {
		market       Market
		zone         string
		closeTime    string
		winterOffset int
		summerOffset int
	}{
		{MarketAStock, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketIndex, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketEtfLof, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketBond, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketOther, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketBlock, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
		{MarketFuture, "Asia/Shanghai", "15:30", 8 * 3600, 8 * 3600},
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
