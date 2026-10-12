package tdx

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/ios/client"
)

func poolTestClient(closed *atomic.Int32) *StConn {
	c := client.New(nil)
	c.Closer.Reset()
	c.SetCloseFunc(func(error) error { closed.Add(1); return nil })
	return newStConn(&conn{Client: c})
}

func TestPoolCloseAndReturn(t *testing.T) {
	for i := 0; i < 100; i++ {
		var closed atomic.Int32
		p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 2)
		if err != nil {
			t.Fatal(err)
		}
		borrowed, err := p.Get()
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
		if closed.Load() != 1 {
			t.Fatal("idle connection was not closed", closed.Load())
		}
		p.Put(borrowed)
		if closed.Load() != 2 {
			t.Fatal("returned connection was not closed", closed.Load())
		}
		if c, err := p.Get(); err == nil || c != nil {
			t.Fatal("closed pool returned a connection")
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPoolConcurrentCloseAndReturn(t *testing.T) {
	var closed atomic.Int32
	const n = 32
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, n)
	if err != nil {
		t.Fatal(err)
	}
	var clients []Client
	for i := 0; i < n; i++ {
		c, err := p.Get()
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, c)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, c := range clients {
		wg.Add(1)
		go func(c Client) { defer wg.Done(); <-start; p.Put(c) }(c)
	}
	close(start)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if closed.Load() != n {
		t.Fatalf("closed %d/%d connections", closed.Load(), n)
	}
}

func TestPoolCloseDuringDo(t *testing.T) {
	var closed atomic.Int32
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- p.Do(func(Client) error { close(entered); <-finish; return nil })
	}()
	<-entered
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 0 {
		t.Fatal("borrowed connection closed before callback ended")
	}
	close(finish)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not finish after pool was closed")
	}
	if closed.Load() != 1 {
		t.Fatal("borrowed connection leaked")
	}
}

func TestPoolDialFailureClosesExistingClients(t *testing.T) {
	var closed atomic.Int32
	want := errors.New("dial failed")
	calls := 0
	p, err := NewPool(func() (Client, error) {
		calls++
		if calls == 2 {
			return nil, want
		}
		return poolTestClient(&closed), nil
	}, 2)
	if p != nil || !errors.Is(err, want) {
		t.Fatal(p, err)
	}
	if closed.Load() != 1 {
		t.Fatal("successful dial leaked after initialization failed")
	}
}

func TestPoolRejectsNilClientAndClosesExisting(t *testing.T) {
	for _, empty := range []Client{nil, (*StConn)(nil), (*ExConn)(nil)} {
		var closed atomic.Int32
		calls := 0
		p, err := NewPool(func() (Client, error) {
			calls++
			if calls == 2 {
				return empty, nil
			}
			return poolTestClient(&closed), nil
		}, 2)
		if p != nil || err == nil || closed.Load() != 1 {
			t.Fatal("nil factory result was accepted or initialization leaked", p, err, closed.Load())
		}
	}
}
