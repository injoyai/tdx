package protocol

import "testing"

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
