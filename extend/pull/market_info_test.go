package pull

import (
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestMarketInfoRouting(t *testing.T) {
	for _, tc := range []struct {
		code             string
		market           Market
		protocol         MarketProtocol
		kind             string
		day, min         uint8
		dayUnit, minUnit VolumeUnit
	}{
		{"sz000001", MarketAStock, ProtocolStandard, protocol.KindStock, 9, 7, VolumeHundredShares, VolumeHundredShares},
		{"sh510300", MarketEtfLof, ProtocolStandard, protocol.KindStock, 9, 7, VolumeHundredShares, VolumeHundredShares},
		{"sh000001", MarketIndex, ProtocolStandard, protocol.KindIndex, 9, 7, VolumeShares, VolumeShares},
		{"sh880001", MarketBlock, ProtocolStandard, protocol.KindIndex, 9, 7, VolumeShares, VolumeShares},
		{"hk00700", MarketHK, ProtocolExtended, "", 4, 8, VolumeHundredShares, VolumeDefault},
		{"cffIF2609", MarketFuture, ProtocolExtended, "", 4, 8, VolumeDefault, VolumeDefault},
		{"hk.HSI", MarketHKIndex, ProtocolExtended, "", 9, 8, VolumeDefault, VolumeDefault},
		{"usAAPL", MarketUS, ProtocolExtended, "", 9, 8, VolumeDefault, VolumeDefault},
	} {
		t.Run(tc.code, func(t *testing.T) {
			code, err := ParseCode(tc.code)
			if err != nil || code.Market != tc.market {
				t.Fatal("market routing", code, err)
			}
			info, ok := code.Market.Info()
			if !ok || info.Protocol != tc.protocol || info.Kind != tc.kind ||
				info.Day != (KlineInfo{Category: tc.day, VolumeUnit: tc.dayUnit}) || info.Minute != (KlineInfo{Category: tc.min, VolumeUnit: tc.minUnit}) {
				t.Fatal("unexpected market configuration", info, ok)
			}
		})
	}
	if _, ok := Market("custom").Info(); ok {
		t.Fatal("custom market received a built-in configuration")
	}
	info, _ := MarketHK.Info()
	info.Day.Category = 9
	unchanged, _ := MarketHK.Info()
	if unchanged.Day.Category != 4 {
		t.Fatal("editing a copy changed the built-in configuration")
	}
	for _, unit := range []VolumeUnit{VolumeDefault, VolumeHundredShares, ""} {
		if got := (KlineInfo{VolumeUnit: unit}).ToStorageVolume(123); got != 12300 {
			t.Fatal("default conversion", unit, got)
		}
	}
}

func TestStandardIndexVolumeIsNotConvertedTwice(t *testing.T) {
	info, _ := MarketIndex.Info()
	for _, spec := range []KlineInfo{info.Day, info.Minute} {
		// 一条指数记录：价格差值为零，原始量 123400，额和涨跌家数为零。
		data := make([]byte, 22)
		binary.LittleEndian.PutUint16(data, 1)
		binary.LittleEndian.PutUint32(data[2:6], 20261009)
		binary.LittleEndian.PutUint32(data[10:14], math.Float32bits(123400))
		bars, err := protocol.MKline.Decode(data, protocol.KlineCache{Type: spec.Category, Kind: info.Kind, Code: "sh000001"})
		if err != nil || len(bars.List) != 1 {
			t.Fatal(bars, err)
		}
		// 日线原始量是手；分钟原始量是股，解码器先除 100 再乘 100。
		want := int64(123400)
		if spec.Category == protocol.TypeKlineDay {
			want *= 100
		}
		if got := spec.ToStorageVolume(bars.List[0].Volume); got != want {
			t.Fatalf("category %d: stored volume %d, want %d", spec.Category, got, want)
		}
	}
}

func TestHKDailyCapturedVolume(t *testing.T) {
	// 2026-10-11 捕获的 7727 原始响应，00700，category=4；单根与五根请求。
	const single = "1f30303730300000000004000100000000000100912835010180cf4367e6d4436766cf436766d443064e0050c11d0300d07c0f4a"
	const five = "1f303037303000000000040001000000000005008d2835010100d243cdccd3433433d0430180d3436c925a4f3654010020ba4f4a8e2835013433d5439a99d643cdccd4439a19d643f1797f4fcd870100f006064a8f283501cdccd4430100d5439a99d143cd4cd24389289c4fcee50100f014254a902835010100d343cd4cd4439a99cd4334b3cd43771b3b50cd9b04000086a14a912835010180cf4367e6d4436766cf436766d443064e0050c11d0300d07c0f4a"
	// 腾讯独立源的不复权日线（2026-10-05 至 10-09），来自：
	// https://web.ifzq.gtimg.cn/appstock/app/fqkline/get?param=hk00700,day,2026-10-05,2026-10-09,5,
	shares := []int64{8709438, 10030081, 12436640, 30202875, 20422500}
	info, _ := MarketHK.Info()
	var latest protocol.ExKline
	for _, raw := range []string{five, single} {
		data, err := hex.DecodeString(raw)
		if err != nil {
			t.Fatal(err)
		}
		if uint8(binary.LittleEndian.Uint16(data[10:12])) != info.Day.Category {
			t.Fatal("request category differs from captured response")
		}
		bars, err := protocol.MEx.DecodeBars(data, protocol.ExBarsCache{Category: info.Day.Category})
		if err != nil || len(bars) != len(data[20:])/32 {
			t.Fatal("decode captured response", bars, err)
		}
		if len(bars) == 5 {
			latest = bars[4]
			for i, bar := range bars {
				got := info.Day.ToStorageVolume(int64(bar.Trade))
				if delta := got - shares[i]; delta < -50 || delta > 50 {
					t.Fatalf("%s: %d shares, independent source %d", bar.Datetime, got, shares[i])
				}
			}
		} else if bars[0] != latest || info.Day.ToStorageVolume(int64(bars[0].Trade)) != 20422500 {
			t.Fatal("single-record volume differs from multi-record volume", bars)
		}
	}
}
