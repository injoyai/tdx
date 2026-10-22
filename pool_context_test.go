package tdx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolContextCancellation(t *testing.T) {
	var closed atomic.Int32
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	held, err := p.Get()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- p.DoContext(ctx, func(Client) error { t.Error("canceled waiter called callback"); return nil })
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled waiter did not exit")
	}
	p.Put(held)
	if err := p.DoContext(ctx, func(Client) error { t.Error("already canceled request borrowed idle connection"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := p.Do(func(Client) error { return nil }); err != nil {
		t.Fatal("cancellation lost connection", err)
	}
}
