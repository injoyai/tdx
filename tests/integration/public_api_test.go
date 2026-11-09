package integration_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestConnectionHidesTransportState(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf((*tdx.StConn)(nil)), reflect.TypeOf((*tdx.ExConn)(nil))} {
		for _, name := range []string{"Read", "Reset", "Restart", "Run", "SetFunc", "SetReadWriteCloser", "CloseAll", "WithFrame"} {
			if _, ok := typ.MethodByName(name); ok {
				t.Errorf("%s still exposes %s", typ, name)
			}
		}
		for _, name := range []string{"Client", "Wait", "MoreWriter", "Logger", "Event"} {
			if _, ok := typ.Elem().FieldByName(name); ok {
				t.Errorf("%s still exposes mutable %s", typ, name)
			}
		}
		for _, name := range []string{"SendFrame", "Write", "WriteString", "WriteHEX", "Done", "Closed", "Err", "GetKey"} {
			if _, ok := typ.MethodByName(name); !ok {
				t.Errorf("%s lost supported %s", typ, name)
			}
		}
	}
}

func TestRiseRateUsesRatioAndAvailability(t *testing.T) {
	for _, tc := range []struct {
		base, close float64
		available   bool
	}{
		{10, 11, true}, {10, 10, true}, {10, 9, true}, {0, 1, false},
		{10, math.NaN(), false}, {10, math.Inf(1), false}, {math.SmallestNonzeroFloat64, math.MaxFloat64, false},
	} {
		k := &protocol.Kline{Last: tc.base, Close: tc.close}
		i := &extend.Income{Source: protocol.Kline{Close: tc.base}, Current: protocol.Kline{Close: tc.close}}
		a, okA := k.RiseRate()
		b, okB := i.RiseRate()
		if okA != tc.available || okA != okB || a != b {
			t.Fatal("cross-package rate differs", a, okA, b, okB)
		}
	}
	k := &protocol.Kline{Open: 10, Close: 11}
	if rate, ok := k.RiseRate(); !ok || rate != .1 {
		t.Fatal("open fallback changed", rate, ok)
	}
	if rate, ok := (*protocol.Kline)(nil).RiseRate(); ok || rate != 0 {
		t.Fatal("nil rate available")
	}
}

// A variadic Update method is adapted without an object-specific Updater interface.
func scheduleService(s *pull.Service) {
	_, _ = update.StartTimer("0 0 9 * * *", 1, func() error { return s.Update() })
}
