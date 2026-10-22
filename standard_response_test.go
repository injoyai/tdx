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
