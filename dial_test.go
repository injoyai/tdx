package tdx

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
)

func localDialListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

func TestDialExplicitPort(t *testing.T) {
	l := localDialListener(t)
	addr := l.Addr().String()
	for name, dial := range map[string]ios.DialFunc{
		"tcp":    NewTCPDial(addr),
		"host":   NewHostDial(Addrs{addr}),
		"random": NewRandomDial(Addrs{addr}),
		"range":  NewRangeDial(Addrs{addr}),
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c, key, err := dial(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if key != addr {
				t.Fatalf("dial address = %q, want %q", key, addr)
			}
		})
	}
}

func TestDialRequiresPort(t *testing.T) {
	addr := "127.0.0.1"
	for name, dial := range map[string]ios.DialFunc{
		"tcp":    NewTCPDial(addr),
		"host":   NewHostDial(Addrs{addr}),
		"random": NewRandomDial(Addrs{addr}),
		"range":  NewRangeDial(Addrs{addr}),
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			c, _, err := dial(ctx)
			if c != nil {
				c.Close()
				t.Fatal("bare IP established a connection")
			}
			var addressError *net.AddrError
			if !errors.As(err, &addressError) || addressError.Addr != addr {
				t.Fatalf("bare IP must fail address validation, got %v", err)
			}
		})
	}
}

func TestStandardDialHostsDefaults(t *testing.T) {
	previous := Hosts
	Hosts = Addrs{localDialListener(t).Addr().String()}
	t.Cleanup(func() { Hosts = previous })
	for name, dial := range map[string]func([]string, ...client.Option) (*StConn, error){
		"host": DialHosts, "random": DialHostsRandom, "range": DialHostsRange,
	} {
		t.Run(name, func(t *testing.T) {
			for _, hosts := range [][]string{nil, {}} {
				c, err := dial(hosts, WithRedial(false), WithLevel(LevelNone), WithDebug(false))
				if err != nil {
					t.Fatal("standard defaults were not selected:", err)
				}
				if err := c.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	Hosts = nil
	for _, dial := range []func([]string, ...client.Option) (*StConn, error){DialHosts, DialHostsRandom, DialHostsRange} {
		c, err := dial(nil, WithRedial(false), WithLevel(LevelNone), WithDebug(false))
		if c != nil || err == nil {
			if c != nil {
				c.Close()
			}
			t.Fatal("empty standard defaults must return an error", c, err)
		}
	}
}

func TestHostDialRotation(t *testing.T) {
	first, second := localDialListener(t), localDialListener(t)
	hosts := []string{first.Addr().String(), second.Addr().String()}
	dial := NewHostDial(hosts)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, want := range []string{hosts[0], hosts[1], hosts[0]} {
		c, key, err := dial(ctx)
		if err != nil {
			t.Fatal(err)
		}
		c.Close()
		if key != want {
			t.Fatalf("dial address = %q, want %q", key, want)
		}
	}
}

func TestHostAddrsPreservesPortsAndInput(t *testing.T) {
	hosts := []string{"127.0.0.1", "127.0.0.1:12345", "[::1]:12345", "::1", "[::1]", "fe80::1%lo0", "[::1", "::1]"}
	for _, port := range []string{stPort, ExPort} {
		want := []string{"127.0.0.1:" + port, hosts[1], hosts[2], "[::1]:" + port, "[::1]:" + port, "[fe80::1%lo0]:" + port, hosts[6], hosts[7]}
		if got := hostAddrs(hosts, nil, port); !reflect.DeepEqual(got, want) {
			t.Fatalf("addresses = %v, want %v", got, want)
		}
		if got := hostAddrs(nil, hosts, port); !reflect.DeepEqual(got, want) {
			t.Fatalf("default addresses = %v, want %v", got, want)
		}
	}
	if hosts[0] != "127.0.0.1" {
		t.Fatal("normalization modified caller's hosts")
	}
}

func TestSharedDialFunctions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hosts := []string{"127.0.0.1:1", "127.0.0.1:2"}
	for name, dial := range map[string]ios.DialFunc{"host": NewHostDial(hosts), "random": NewRandomDial(hosts), "tcp": NewTCPDial(hosts[0])} {
		t.Run(name, func(t *testing.T) {
			var wg sync.WaitGroup
			for i := 0; i < 4; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := 0; j < 100; j++ {
						c, key, err := dial(ctx)
						if c != nil {
							c.Close()
						}
						if !errors.Is(err, context.Canceled) || (key != hosts[0] && key != hosts[1]) {
							t.Error("canceled dial", key, err)
						}
					}
				}()
			}
			wg.Wait()
		})
	}
}

func TestDialExHostsUsesExtendedDefaults(t *testing.T) {
	l := localDialListener(t)
	previous, previousEx := Hosts, ExHosts
	Hosts, ExHosts = []string{"127.0.0.1:0"}, []string{l.Addr().String()}
	t.Cleanup(func() { Hosts, ExHosts = previous, previousEx })
	for _, hosts := range [][]string{nil, {}, ExHosts} {
		c, err := DialExHosts(hosts, WithRedial(false), WithLevel(LevelNone), WithDebug(false))
		if err != nil {
			t.Fatal("extended dial used the wrong address or port:", err)
		}
		if err := c.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDialEmptyHosts(t *testing.T) {
	previous, previousEx := Hosts, ExHosts
	Hosts, ExHosts = Addrs{localDialListener(t).Addr().String()}, nil
	t.Cleanup(func() { Hosts, ExHosts = previous, previousEx })
	for _, factory := range []func([]string) ios.DialFunc{NewHostDial, NewRandomDial, NewRangeDial} {
		c, _, err := factory(nil)(context.Background())
		if c != nil || err == nil {
			t.Fatal("empty hosts must return a dial error", c, err)
		}
	}
	// 扩展默认列表为空时，即使标准地址可用也不能回退到标准协议。
	if c, err := DialExHosts(nil, WithRedial(false), WithLevel(LevelNone), WithDebug(false)); c != nil || err == nil {
		if c != nil {
			c.Close()
		}
		t.Fatal("empty extended hosts fell back to standard hosts", c, err)
	}
}

func TestDialContextCancelsHandshake(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return a, "pipe", nil }
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				var err error
				if ex {
					_, err = dialEx(ctx, dial, WithRedial(false), WithDebug(false))
				} else {
					_, err = dialSt(ctx, dial, WithRedial(false), WithDebug(false))
				}
				finished <- err
			}()
			select {
			case err := <-finished:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("handshake ignored context", err)
				}
			case <-time.After(time.Second):
				t.Fatal("handshake hung after cancellation")
			}
		})
	}
}

func TestCanceledDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		factory func([]string) ios.DialFunc
	}{{"host", NewHostDial}, {"random", NewRandomDial}, {"range", NewRangeDial}} {
		t.Run(test.name, func(t *testing.T) {
			c, _, err := test.factory([]string{listener.Addr().String()})(ctx)
			if c != nil {
				defer c.Close()
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("canceled context still established TCP connection: error=%v", err)
			}
		})
	}
}

func TestRangeDialCancelsRetryWait(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	start := time.Now()
	c, _, err := NewRangeDial([]string{addr, addr})(ctx)
	cancel()
	if c != nil {
		c.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("retry wait ignored context", err, time.Since(start))
	}
}
