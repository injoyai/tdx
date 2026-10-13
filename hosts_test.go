package tdx

import (
	"context"
	"net"
	"slices"
	"testing"
	"time"
)

func TestAddrsSortUnavailable(t *testing.T) {
	done := make(chan []string, 1)
	go func() { done <- (Addrs{"127.0.0.1:0"}).Sort(20 * time.Millisecond) }()
	select {
	case hosts := <-done:
		if len(hosts) != 0 {
			t.Fatalf("unavailable hosts returned: %v", hosts)
		}
	case <-time.After(time.Second):
		t.Fatal("host sorting did not return after its timeout with no reachable server")
	}
}

func TestAddrsSortFiltersUnavailable(t *testing.T) {
	listener := localDialListener(t)
	addr := listener.Addr().String()
	input := Addrs{"127.0.0.1:0", addr}
	before := slices.Clone(input)
	standard, extended := slices.Clone(Hosts), slices.Clone(ExHosts)
	hosts := input.Sort(time.Second)
	if len(hosts) != 1 || hosts[0] != addr {
		t.Fatalf("available hosts = %v, want [%s]", hosts, addr)
	}
	hosts[0] = "changed-result"
	if !slices.Equal(input, before) || !slices.Equal(Hosts, standard) || !slices.Equal(ExHosts, extended) {
		t.Fatal("sorting or modifying the result changed the input or default lists")
	}
	if hosts := Addrs(nil).Sort(time.Second); len(hosts) != 0 {
		t.Fatalf("empty input returned hosts: %v", hosts)
	}
	if hosts := (Addrs{addr}).Sort(0); len(hosts) != 0 {
		t.Fatalf("expired deadline returned hosts: %v", hosts)
	}
}

func TestAddrsSortMixedPorts(t *testing.T) {
	first, second := localDialListener(t), localDialListener(t)
	input := Addrs{first.Addr().String(), second.Addr().String()}
	hosts := input.Sort()
	if len(hosts) != 2 || !slices.Contains(hosts, input[0]) || !slices.Contains(hosts, input[1]) {
		t.Fatalf("mixed ports returned %v, want both addresses %v", hosts, input)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, key, err := NewRangeDial(hosts)(ctx)
	if err != nil {
		t.Fatal("sorted Addrs could not be passed to the dialer:", err)
	}
	defer c.Close()
	if key != hosts[0] {
		t.Fatalf("dial address = %q, want fastest address %q", key, hosts[0])
	}
}

func TestBuiltinHostPorts(t *testing.T) {
	for _, test := range []struct {
		hosts []string
		port  string
	}{
		{Hosts, "7709"}, {SHHosts, "7709"}, {BJHosts, "7709"}, {GZHosts, "7709"}, {WHHosts, "7709"},
		{ExHosts, ExPort}, {ExSHHosts, ExPort}, {ExBJHosts, ExPort}, {ExGZHosts, ExPort},
	} {
		for _, addr := range test.hosts {
			host, port, err := net.SplitHostPort(addr)
			if err != nil || net.ParseIP(host) == nil || port != test.port {
				t.Errorf("invalid built-in address %q: want IP:%s", addr, test.port)
			}
		}
	}
}
