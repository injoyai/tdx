package tdx

import (
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestMinuteStandardResponse(t *testing.T) {
	p := &protocol.PriceNumber{Time: "09:31", Price: 12345, Number: 7}
	r := &MinuteResp{Count: 99, List: []Minute{{Standard: p}}}
	out, err := r.StandardResponse()
	if err != nil || out.Count != 1 || len(out.List) != 1 || out.List[0] != *p {
		t.Fatal(out, err)
	}
	for _, r := range []*MinuteResp{
		{Instrument: protocol.Instrument{Exchange: protocol.ExchangeHK}},
		{List: []Minute{{}}},
		{List: make([]Minute, 65536)},
	} {
		if out, err := r.StandardResponse(); out != nil || err == nil {
			t.Fatal("invalid conversion accepted", out, err)
		}
	}
	out, err = (&MinuteResp{}).StandardResponse()
	if err != nil || out.Count != 0 || len(out.List) != 0 {
		t.Fatal("empty result changed", out, err)
	}
}

func TestKlinesVolumeValidation(t *testing.T) {
	for _, tc := range []struct {
		unit   protocol.VolumeUnit
		volume int64
		want   int64
		valid  bool
	}{
		{protocol.VolumeHundredShares, 123, 123, true},
		{protocol.VolumeShares, 12300, 123, true},
		{protocol.VolumeShares, 12301, 0, false},
		{VolumeUnknown, 123, 0, false},
		{"", 123, 0, false},
	} {
		r := &KlineResp{VolumeUnit: tc.unit, List: []Kline{{Standard: &protocol.Kline{Volume: tc.volume}}}}
		ks, err := r.Klines()
		if (err == nil) != tc.valid {
			t.Fatalf("unit=%s volume=%d: %v", tc.unit, tc.volume, err)
		}
		if tc.valid && (ks[0].Volume != tc.want || r.List[0].Standard.Volume != tc.volume) {
			t.Fatal("incorrect or mutating conversion", ks)
		}
	}
}
