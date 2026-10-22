package gbbq

import (
	"reflect"
	"testing"

	"github.com/injoyai/tdx/v2"
)

func TestClientInterfaceNilInjection(t *testing.T) {
	if reflect.TypeOf((*tdx.Client)(nil)).Elem().Kind() != reflect.Interface {
		t.Fatal("tdx.Client is not an interface")
	}
	for _, api := range []tdx.Client{(*tdx.StConn)(nil), (*tdx.MarketClient)(nil), nil} {
		var g Gbbq
		WithClient(api)(&g)
		if g.c != nil {
			t.Fatal("typed nil client no longer follows nil injection behavior")
		}
	}
	dial := func() (tdx.Client, error) { return (*tdx.StConn)(nil), nil }
	if g, err := New(WithDialClient(dial)); g != nil || err == nil {
		t.Fatal("nil factory accepted", g, err)
	}
}

func TestGbbqAcceptsUnifiedClient(t *testing.T) {
	api := tdx.NewClient()
	defer api.Close()
	var g Gbbq
	WithClient(api)(&g)
	if g.c != api {
		t.Fatal("metadata manager requires a concrete connection")
	}
}
