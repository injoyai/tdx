package manage

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
)

type closingCodes struct {
	*codes.Base
	closed atomic.Int32
}

func (c *closingCodes) Close() error { c.closed.Add(1); return nil }

func TestManageCloseOwnership(t *testing.T) {
	for _, own := range []bool{false, true} {
		var poolClosed atomic.Int32
		p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&poolClosed), nil }, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		cache := &closingCodes{Base: codes.NewBase()}
		var workdayClosed, equityClosed atomic.Int32
		w := testWorkday(t, &workdayClosed)
		g := testGbbq(t, &equityClosed)
		opts := []Option{WithPool(p), WithCodes(cache), WithWorkday(w), WithGbbq(g)}
		if own {
			opts = []Option{WithDialPool(func() (tdx.ClientPool, error) { return p, nil }), WithDialCodes(func(tdx.Client) (codes.Interface, error) { return cache, nil }), WithDialWorkday(func(tdx.Client) (*workday.Workday, error) { return w, nil }), WithDialGbbq(func(tdx.Client) (gbbq.Interface, error) { return g, nil })}
		}
		m, err := New(opts...)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.AddWorkdayTask("@every 87600h", func(*Manage) {}); err != nil {
			t.Fatal(err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		if err := m.AddWorkdayTask("@every 87600h", func(*Manage) {}); !errors.Is(err, net.ErrClosed) {
			t.Fatal("added task after close", err)
		}
		if _, err := m.InitGbbq(); !errors.Is(err, net.ErrClosed) {
			t.Fatal("initialized metadata after close", err)
		}
		want := int32(0)
		if own {
			want = 1
		}
		if poolClosed.Load() != want || cache.closed.Load() != want || workdayClosed.Load() != want || equityClosed.Load() != want {
			t.Fatal("ownership violated", own, poolClosed.Load(), cache.closed.Load(), workdayClosed.Load())
		}
	}
}

func TestManageFailureClosesCreatedCache(t *testing.T) {
	var closed atomic.Int32
	p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	cache := &closingCodes{Base: codes.NewBase()}
	want := errors.New("workday initialization failed")
	m, err := New(WithPool(p), WithDialCodes(func(tdx.Client) (codes.Interface, error) { return cache, nil }), WithDialWorkday(func(tdx.Client) (*workday.Workday, error) { return nil, want }))
	if m != nil || !errors.Is(err, want) || cache.closed.Load() != 1 || closed.Load() != 0 {
		t.Fatal("initialization cleanup", m, err, cache.closed.Load(), closed.Load())
	}
}

func TestManageCloseUnblocksTaskWaitingForPool(t *testing.T) {
	var closed atomic.Int32
	p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	w := testWorkday(t, new(atomic.Int32))
	m, err := New(WithDialPool(func() (tdx.ClientPool, error) { return p, nil }), WithCodes(codes.NewBase()), WithWorkday(w))
	if err != nil {
		t.Fatal(err)
	}
	borrowed, err := p.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Put(borrowed)
	entered := make(chan struct{}, 1)
	result := make(chan error, 1)
	if err := m.AddWorkdayTask("@every 1s", func(m *Manage) {
		select {
		case entered <- struct{}{}:
		default:
		}
		result <- m.Do(func(tdx.Client) error { t.Error("obtained exhausted connection"); return nil })
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("task did not start")
	}
	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		p.Close()
		<-done
		t.Fatal("close waited for exhausted pool")
	}
	if err := <-result; err == nil {
		t.Fatal("waiting acquisition succeeded after close")
	}
	if closed.Load() != 0 {
		t.Fatal("borrowed client closed before return")
	}
}

func TestManageCloseWaitsForTask(t *testing.T) {
	var closed atomic.Int32
	p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	w := testWorkday(t, new(atomic.Int32))
	m, err := New(WithDialPool(func() (tdx.ClientPool, error) { return p, nil }), WithCodes(codes.NewBase()), WithWorkday(w))
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}, 1), make(chan struct{})
	if err := m.AddWorkdayTask("@every 1s", func(*Manage) { entered <- struct{}{}; <-release }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		m.Close()
		t.Fatal("task did not start")
	}
	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	select {
	case <-done:
		close(release)
		t.Fatal("close did not wait for active task")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
}
