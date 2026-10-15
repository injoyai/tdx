package tdx

import (
	"errors"
	"github.com/injoyai/base/maps"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type closingCodes struct {
	*CodesBase
	closed atomic.Int32
}

func (c *closingCodes) Close() error { c.closed.Add(1); return nil }

func TestManageCloseOwnership(t *testing.T) {
	for _, own := range []bool{false, true} {
		var poolClosed atomic.Int32
		p, err := NewPool(func() (Client, error) { return poolTestClient(&poolClosed), nil }, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		codes := &closingCodes{CodesBase: NewCodesBase()}
		w := &Workday{}
		var workdayClosed, equityClosed atomic.Int32
		g := &Gbbq{}
		g.updates.cleanup = func() error { equityClosed.Add(1); return nil }
		w.updates.cleanup = func() error { workdayClosed.Add(1); return nil }
		opts := []Option{WithPool(p), WithCodes(codes), WithWorkday(w), WithGbbq(g)}
		if own {
			opts = []Option{WithDialPool(func() (IPool, error) { return p, nil }), WithDialCodes(func(Client) (ICodes, error) { return codes, nil }), WithDialWorkday(func(Client) (*Workday, error) { return w, nil }), WithDialGbbq(func(Client) (IGbbq, error) { return g, nil })}
		}
		m, err := NewManage(opts...)
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
		if poolClosed.Load() != want || codes.closed.Load() != want || workdayClosed.Load() != want || equityClosed.Load() != want {
			t.Fatal("ownership violated", own, poolClosed.Load(), codes.closed.Load(), workdayClosed.Load())
		}
	}
}

func TestManageFailureClosesCreatedCache(t *testing.T) {
	var closed atomic.Int32
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	codes := &closingCodes{CodesBase: NewCodesBase()}
	want := errors.New("workday initialization failed")
	m, err := NewManage(WithPool(p), WithDialCodes(func(Client) (ICodes, error) { return codes, nil }), WithDialWorkday(func(Client) (*Workday, error) { return nil, want }))
	if m != nil || !errors.Is(err, want) || codes.closed.Load() != 1 || closed.Load() != 0 {
		t.Fatal("initialization cleanup", m, err, codes.closed.Load(), closed.Load())
	}
}

func TestManageCloseUnblocksTaskWaitingForPool(t *testing.T) {
	var closed atomic.Int32
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	w := &Workday{cache: maps.NewBit()}
	w.cache.Set(uint64(IntegerDay(time.Now()).Add(15*time.Hour).Unix()), true)
	m, err := NewManage(WithDialPool(func() (IPool, error) { return p, nil }), WithCodes(NewCodesBase()), WithWorkday(w))
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
		result <- m.Do(func(Client) error { t.Error("obtained exhausted connection"); return nil })
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
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	w := &Workday{cache: maps.NewBit()}
	w.cache.Set(uint64(IntegerDay(time.Now()).Add(15*time.Hour).Unix()), true)
	m, err := NewManage(WithDialPool(func() (IPool, error) { return p, nil }), WithCodes(NewCodesBase()), WithWorkday(w))
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
