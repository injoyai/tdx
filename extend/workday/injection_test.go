package workday

import (
	"github.com/injoyai/tdx/v2"
	"testing"
)

func TestWorkdayAcceptsUnifiedClient(t *testing.T) {
	api := tdx.NewClient()
	defer api.Close()
	var w Workday
	WithClient(api)(&w)
	if w.c != api {
		t.Fatal("calendar requires a concrete connection")
	}
}

func TestWorkdayClientNilInjection(t *testing.T) {
	for _, api := range []tdx.Client{(*tdx.StConn)(nil), (*tdx.MarketClient)(nil), nil} {
		var w Workday
		WithClient(api)(&w)
		if w.c != nil {
			t.Fatal("typed nil client no longer follows nil injection behavior")
		}
	}
}
