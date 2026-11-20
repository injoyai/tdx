package protocol

import (
	"encoding/hex"
	"math"
	"testing"
	"time"
)

func TestStockKlineFrame(t *testing.T) {
	//预期0c02000000001c001c002d050000303030303031 0900 0100 0000 0a00 00000000000000000000
	//   0c00000000011c001c002d050000313030303030 0900 0000 0000 0a00 00000000000000000000
	f, _ := MKline.Frame(TypeKlineDay, "sz000001", 0, 10)
	t.Log(f.Bytes().HEX())
}

func TestStockKlineDecode(t *testing.T) {
	s := "0a0078da340198b8018404bc055ee8b3e949ad2b094f79da34010af801a002cc0260dec949859ded4e7ada34016882028e04e603b8f91e4a111f394f7dda3401e401c20200f604f84d2b4ad4d0444f7eda3401721eaa0268d87bc549ee80e34e7fda34011e288601c601d08db849230ed54e80da3401727c32da013023584999a0784e81da3401147c0ad001d0fa86498d989a4e84da34015e6800d60278c28e491ca6a14e85da340154d001b801da01403e924989d6a54e"
	bs, err := hex.DecodeString(s)
	if err != nil {
		t.Error(err)
		return
	}
	resp, err := MKline.Decode(bs, KlineCache{
		Type: 9,
		Kind: "",
	})
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(len(resp.List))
	for _, v := range resp.List {
		t.Log(v)
	}
}

func TestNormalizeKlineVolume(t *testing.T) {
	tests := []struct {
		name   string
		volume int64
		cache  KlineCache
		want   int64
	}{
		{name: "stock minute", volume: 43600, cache: KlineCache{Type: TypeKlineMinute, Code: "sh600000"}, want: 436},
		{name: "sh convertible bond minute", volume: 436, cache: KlineCache{Type: TypeKlineMinute, Code: "sh110075"}, want: 436},
		{name: "sz convertible bond minute", volume: 534, cache: KlineCache{Type: TypeKlineMinute, Code: "sz123064"}, want: 534},
		{name: "stock day", volume: 43600, cache: KlineCache{Type: TypeKlineDay, Code: "sh600000"}, want: 43600},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := standardMarketInfo(test.cache.Code)
			if got := info.StandardKlineVolume(test.volume, test.cache.Type, test.cache.Kind); got != test.want {
				t.Fatalf("standardKlineVolume() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestStandardKlineBusinessVolume(t *testing.T) {
	tests := []struct {
		name     string
		volume   int64
		code     string
		category uint8
		want     int64
	}{
		{name: "stock minute keeps remainder", volume: 150, code: "sh600000", category: TypeKlineMinute, want: 150},
		{name: "stock day", volume: 150, code: "sh600000", category: TypeKlineDay, want: 15000},
		{name: "convertible bond minute", volume: 150, code: "sh110075", category: TypeKlineMinute, want: 1500},
		{name: "index intraday volume unavailable", volume: 150, code: "sh000001", category: TypeKlineMinute, want: 0},
		{name: "block minute keeps remainder", volume: 150, code: "sh880001", category: TypeKlineMinute, want: 150},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := standardMarketInfo(test.code)
			got, err := info.StandardKlineBusinessVolume(test.volume, test.category, info.Kind)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("StandardKlineBusinessVolume() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestKlineWireVolumePreserved(t *testing.T) {
	b := Bytes(uint16(1))
	b = append(b, Bytes(uint16((2026-2004)*2048+10*100+9))...)
	b = append(b, Bytes(uint16(572))...) // 09:32
	b = append(b, 0x90, 0x9c, 1)
	b = append(b, 0, 0, 0)
	b = append(b, Bytes(math.Float32bits(150))...)
	b = append(b, Bytes(math.Float32bits(1500))...)
	resp, err := MKline.Decode(b, KlineCache{Type: TypeKlineMinute, Kind: KindStock, Code: "sh600000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.List) != 1 {
		t.Fatalf("got %d records", len(resp.List))
	}
	k := resp.List[0]
	if k.Volume != 1 || k.WireVolume() != 150 || k.Amount != 1500 {
		t.Fatalf("Volume=%d WireVolume=%d Amount=%v", k.Volume, k.WireVolume(), k.Amount)
	}
	if k.Time.Format(time.TimeOnly) != "09:32:00" {
		t.Fatalf("unexpected time %s", k.Time)
	}
}

func TestMerge241Periods(t *testing.T) {
	day := time.Date(2026, 1, 5, 9, 30, 0, 0, time.Local)
	ks := Klines{{Time: day, Open: 1, High: 1, Low: 1, Close: 1, Volume: 10}}
	for i := 1; i <= 5; i++ {
		ks = append(ks, &Kline{Time: day.Add(time.Duration(i) * time.Minute), Open: 1, High: 1.1, Low: 0.9, Close: 1.05, Volume: 1})
	}
	for _, n := range []int{-1, 0} {
		if got := ks.Merge241(n); len(got) != len(ks) || got[0] != ks[0] {
			t.Fatal("invalid period changed input", n)
		}
	}
	got := ks.Merge241(5)
	if len(got) != 49 || got[0].Volume != 10 || got[1].Volume != 5 || got[1].Time != day.Add(5*time.Minute) || got[1].High != 1.1 || got[1].Low != 0.9 {
		t.Fatal("normal aggregation changed", got)
	}
	if len(ks) != 6 || ks[1].Volume != 1 {
		t.Fatal("mutated input")
	}
}
