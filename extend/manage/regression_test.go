package manage

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
)

func TestManageFailureCleanup(t *testing.T) {
	want := errors.New("offline initialization failed")
	for _, tc := range []struct {
		name    string
		options []Option
	}{
		{"codes", []Option{WithDialCodes(func(tdx.Client) (codes.Interface, error) { return nil, want })}},
		{"workday", []Option{WithCodes(codes.NewBase()), WithDialWorkday(func(tdx.Client) (*workday.Workday, error) { return nil, want })}},
		{"equity", []Option{WithCodes(codes.NewBase()), WithWorkday(&workday.Workday{}), WithDialGbbq(func(tdx.Client) (gbbq.Interface, error) { return nil, want })}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var closed atomic.Int32
			p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			options := append([]Option{WithDialPool(func() (tdx.ClientPool, error) { return p, nil })}, tc.options...)
			m, err := New(options...)
			if m != nil || !errors.Is(err, want) {
				t.Fatal(m, err)
			}
			if closed.Load() != 1 {
				t.Fatalf("failed initialization closed %d connections; want 1", closed.Load())
			}
		})
	}
}

func TestManagePreservesExternalPoolOnFailure(t *testing.T) {
	var closed atomic.Int32
	p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	want := errors.New("offline codes initialization failed")
	if _, err := New(WithPool(p), WithDialCodes(func(tdx.Client) (codes.Interface, error) { return nil, want })); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if closed.Load() != 0 {
		t.Fatal("caller-owned pool closed")
	}
	c, err := p.Get()
	if err != nil {
		t.Fatal("caller-owned pool unusable", err)
	}
	p.Put(c)
}
