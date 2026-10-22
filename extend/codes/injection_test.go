package codes

import (
	"testing"

	"github.com/injoyai/tdx/v2"
)

func TestCodesAcceptsUnifiedClient(t *testing.T) {
	api := tdx.NewClient()
	defer api.Close()
	var c Codes
	WithClient(api)(&c)
	if c.c != api {
		t.Fatal("cache requires a concrete connection")
	}
}

func TestCodesClientNilInjection(t *testing.T) {
	for _, api := range []tdx.Client{(*tdx.StConn)(nil), (*tdx.MarketClient)(nil), nil} {
		var c Codes
		WithClient(api)(&c)
		if c.c != nil {
			t.Fatal("typed nil client no longer follows nil injection behavior")
		}
	}
	dial := func() (tdx.Client, error) { return (*tdx.StConn)(nil), nil }
	if c, err := New(WithDialClient(dial)); c != nil || err == nil {
		t.Fatal("nil factory accepted", c, err)
	}
}
