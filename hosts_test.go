package tdx

import (
	"net"
	"slices"
	"testing"
	"time"
)

func TestSortHostsUnavailable(t *testing.T) {
	done := make(chan []string, 1)
	go func() { done <- SortHosts([]string{"127.0.0.1:0"}, 20*time.Millisecond) }()
	select {
	case hosts := <-done:
		if len(hosts) != 0 {
			t.Fatalf("unavailable hosts returned: %v", hosts)
		}
	case <-time.After(time.Second):
		t.Fatal("host sorting did not return after its timeout with no reachable server")
	}
}

func TestSortHostsFiltersUnavailable(t *testing.T) {
	listener := localDialListener(t)
	addr := listener.Addr().String()
	input := []string{"127.0.0.1:0", addr}
	before := slices.Clone(input)
	standard, extended := slices.Clone(Hosts), slices.Clone(ExHosts)
	hosts := SortHosts(input, time.Second)
	if len(hosts) != 1 || hosts[0] != addr {
		t.Fatalf("available hosts = %v, want [%s]", hosts, addr)
	}
	hosts[0] = "changed-result"
	if !slices.Equal(input, before) || !slices.Equal(Hosts, standard) || !slices.Equal(ExHosts, extended) {
		t.Fatal("sorting or modifying the result changed the input or default lists")
	}
	if hosts := SortHosts(nil, time.Second); len(hosts) != 0 {
		t.Fatalf("empty input returned hosts: %v", hosts)
	}
	if hosts := SortHosts([]string{addr}, 0); len(hosts) != 0 {
		t.Fatalf("expired deadline returned hosts: %v", hosts)
	}
}

func TestSortHostsMixedPorts(t *testing.T) {
	first, second := localDialListener(t), localDialListener(t)
	input := []string{first.Addr().String(), second.Addr().String()}
	hosts := SortHosts(input)
	if len(hosts) != 2 || !slices.Contains(hosts, input[0]) || !slices.Contains(hosts, input[1]) {
		t.Fatalf("mixed ports returned %v, want both addresses %v", hosts, input)
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
