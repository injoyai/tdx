package protocol

import (
	"encoding/hex"
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
			if got := normalizeKlineVolume(test.volume, test.cache); got != test.want {
				t.Fatalf("normalizeKlineVolume() = %d, want %d", got, test.want)
			}
		})
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
