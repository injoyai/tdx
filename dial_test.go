package tdx

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/ios"
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

func TestDialExplicitPortAndDefaults(t *testing.T) {
	l := localDialListener(t)
	addr := l.Addr().String()
	previous := Hosts
	Hosts = []string{addr}
	t.Cleanup(func() { Hosts = previous })
	for _, factory := range []struct {
		name string
		new  func([]string) ios.DialFunc
	}{{"host", NewHostDial}, {"random", NewRandomDial}, {"range", NewRangeDial}} {
		for _, defaults := range []bool{false, true} {
			hosts := []string{addr}
			name := factory.name + "/explicit"
			if defaults {
				hosts, name = nil, factory.name+"/default"
			}
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				c, key, err := factory.new(hosts)(ctx)
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
	hosts := []string{"127.0.0.1", "127.0.0.1:12345", "[::1]:12345"}
	for _, port := range []string{"7709", ExPort} {
		want := []string{"127.0.0.1:" + port, hosts[1], hosts[2]}
		if got := hostAddrs(hosts, port); !reflect.DeepEqual(got, want) {
			t.Fatalf("addresses = %v, want %v", got, want)
		}
	}
	if hosts[0] != "127.0.0.1" {
		t.Fatal("normalization modified caller's hosts")
	}
}

func TestDialExHqHostsUsesExtendedDefaults(t *testing.T) {
	l := localDialListener(t)
	previous, previousEx := Hosts, ExHosts
	Hosts, ExHosts = []string{"127.0.0.1:0"}, []string{l.Addr().String()}
	t.Cleanup(func() { Hosts, ExHosts = previous, previousEx })
	for _, hosts := range [][]string{nil, {}, ExHosts} {
		c, err := DialExHqHosts(hosts, WithRedial(false), WithLevel(LevelNone), WithDebug(false))
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
	Hosts, ExHosts = nil, nil
	t.Cleanup(func() { Hosts, ExHosts = previous, previousEx })
	for _, factory := range []func([]string) ios.DialFunc{NewHostDial, NewRandomDial, NewRangeDial} {
		c, _, err := factory(nil)(context.Background())
		if c != nil || err == nil {
			t.Fatal("empty hosts must return a dial error", c, err)
		}
	}
	// 扩展默认列表为空时，即使标准地址可用也不能回退到标准协议。
	Hosts = []string{localDialListener(t).Addr().String()}
	if c, err := DialExHqHosts(nil, WithRedial(false), WithLevel(LevelNone), WithDebug(false)); c != nil || err == nil {
		if c != nil {
			c.Close()
		}
		t.Fatal("empty extended hosts fell back to standard hosts", c, err)
	}
}
