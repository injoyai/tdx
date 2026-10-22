package manage

import (
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
)

func TestManageInjectedComponentsDoNotWaitForPool(t *testing.T) {
	var closed atomic.Int32
	p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c, err := p.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Put(c)
	type result struct {
		m   *Manage
		err error
	}
	done := make(chan result, 1)
	go func() {
		m, err := New(WithPool(p), WithCodes(codes.NewBase()), WithWorkday(&workday.Workday{}))
		done <- result{m, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			t.Fatal(r.err)
		}
		if err := r.m.Close(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		p.Close()
		r := <-done
		if r.m != nil {
			r.m.Close()
		}
		t.Fatal("injected components waited for an unused connection")
	}
}

func TestManageRejectsEmptyFactories(t *testing.T) {
	for _, typed := range []bool{false, true} {
		for _, kind := range []string{"pool", "codes", "workday", "gbbq"} {
			name := kind + "/nil"
			if typed {
				name = kind + "/typed-nil"
			}
			t.Run(name, func(t *testing.T) {
				var closed atomic.Int32
				p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
				if err != nil {
					t.Fatal(err)
				}
				defer p.Close()
				cache := &closingCodes{Base: codes.NewBase()}
				opts := []Option{WithDialPool(func() (tdx.ClientPool, error) { return p, nil }), WithDialCodes(func(tdx.Client) (codes.Interface, error) { return cache, nil }), WithWorkday(&workday.Workday{})}
				want := ""
				switch kind {
				case "pool":
					want = "连接池"
					opts = append(opts, WithDialPool(func() (tdx.ClientPool, error) {
						if typed {
							return (*tdx.Pool)(nil), nil
						}
						return nil, nil
					}))
				case "codes":
					want = "代码管理"
					opts = append(opts, WithDialCodes(func(tdx.Client) (codes.Interface, error) {
						if typed {
							return (*closingCodes)(nil), nil
						}
						return nil, nil
					}))
				case "workday":
					want = "工作日管理"
					opts = append(opts, WithDialWorkday(func(tdx.Client) (*workday.Workday, error) { return nil, nil }))
				case "gbbq":
					want = "股本管理"
					opts = append(opts, WithDialGbbq(func(tdx.Client) (gbbq.Interface, error) {
						if typed {
							return (*gbbq.Gbbq)(nil), nil
						}
						return nil, nil
					}))
				}
				m, err := New(opts...)
				if m != nil || err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("got %v, %v", m, err)
				}
				wantPool, wantCodes := int32(1), int32(0)
				if kind == "pool" {
					wantPool = 0
				}
				if kind == "workday" || kind == "gbbq" {
					wantCodes = 1
				}
				if closed.Load() != wantPool || cache.closed.Load() != wantCodes {
					t.Fatalf("cleanup pool=%d codes=%d, want %d %d", closed.Load(), cache.closed.Load(), wantPool, wantCodes)
				}
			})
		}
	}
}

type emptyManagePool struct {
	tdx.ClientPool
	c    tdx.Client
	puts int
}

func (p *emptyManagePool) Get() (tdx.Client, error) { return p.c, nil }
func (p *emptyManagePool) Put(tdx.Client)           { p.puts++ }

func TestManageRejectsEmptyBorrowedClient(t *testing.T) {
	for _, c := range []tdx.Client{nil, (*tdx.StConn)(nil)} {
		p := &emptyManagePool{c: c}
		m, err := New(WithPool(p))
		if m != nil || err == nil || !strings.Contains(err.Error(), "空客户端") || p.puts != 0 {
			t.Fatalf("got %v, %v, puts=%d", m, err, p.puts)
		}
	}
}

func TestManageInitGbbqPreservesInjectedEmptyObject(t *testing.T) {
	for _, factory := range []bool{false, true} {
		var closed atomic.Int32
		p, err := tdx.NewPool(func() (tdx.Client, error) { return poolTestClient(&closed), nil }, 1)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()
		g := &gbbq.Gbbq{}
		opt := WithGbbq(g)
		if factory {
			opt = WithDialGbbq(func(tdx.Client) (gbbq.Interface, error) { return g, nil })
		}
		m, err := New(WithPool(p), WithCodes(codes.NewBase()), WithWorkday(&workday.Workday{}), opt)
		if err != nil {
			t.Fatal(err)
		}
		got, err := m.InitGbbq()
		if err != nil || got != g || !g.IsEmpty() {
			t.Fatalf("injected empty object replaced: %v", err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestManageTaskRequestsExternalClose(t *testing.T) {
	w := testWorkday(t, new(atomic.Int32))
	m := &Manage{Workday: w}
	defer m.Close()
	requested, release, exited := make(chan struct{}, 1), make(chan struct{}), make(chan struct{}, 1)
	if err := m.AddWorkdayTask("@every 1s", func(*Manage) {
		requested <- struct{}{}
		<-release
		exited <- struct{}{}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requested:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("task did not request close")
	}
	done := make(chan error, 1)
	go func() { done <- m.Close() }()
	select {
	case <-done:
		close(release)
		t.Fatal("Close did not wait for task exit")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("external Close did not finish")
	}
	select {
	case <-exited:
	default:
		t.Fatal("Close returned before task exit")
	}
	if err := m.AddWorkdayTask("@every 1s", func(*Manage) {}); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}
