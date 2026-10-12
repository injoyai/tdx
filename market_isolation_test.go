package tdx

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestMarketClientSlowDialKeepsOtherProtocolUsable(t *testing.T) {
	c := newMarketClient()
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { return []byte{2, 0} })
	c.dial[0] = func() (*Conn, error) { return std, nil }
	if _, err := c.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	dialDone := make(chan error, 1)
	c.dial[1] = func() (*Conn, error) {
		close(started)
		<-release
		return nil, io.ErrClosedPipe
	}
	go func() { _, err := c.GetKlineDay("hk00700", 0, 1); dialDone <- err }()
	<-started
	t.Cleanup(func() { close(release); <-dialDone; c.Close() })
	queryDone := make(chan error, 1)
	go func() { _, err := c.GetCount(protocol.ExchangeSZ); queryDone <- err }()
	select {
	case err := <-queryDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow extended dial blocked the existing standard connection")
	}
	setDone := make(chan struct{})
	go func() { c.SetTimeout(3 * time.Second); close(setDone) }()
	select {
	case <-setDone:
	case <-time.After(time.Second):
		t.Fatal("slow extended dial blocked SetTimeout")
	}
}

func TestMarketClientCloseDuringDialDiscardsLateConnection(t *testing.T) {
	c := newMarketClient()
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { return []byte{2, 0} })
	ex := respondingTestClient(t, true, func(_, _ []byte) []byte { return make([]byte, 20) })
	c.dial[0] = func() (*Conn, error) { return std, nil }
	if _, err := c.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	dialDone := make(chan error, 1)
	c.dial[1] = func() (*Conn, error) { close(started); <-release; return ex, nil }
	go func() { _, err := c.GetKlineDay("hk00700", 0, 1); dialDone <- err }()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- c.Close() }()
	var released sync.Once
	releaseDial := func() { released.Do(func() { close(release) }) }
	t.Cleanup(func() { releaseDial(); c.Close() })
	select {
	case <-std.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not release the existing connection during a slow dial")
	}
	select {
	case <-closeDone:
		t.Fatal("Close returned before the pending dial could be cleaned up")
	default:
	}
	releaseDial()
	if err := <-dialDone; !errors.Is(err, net.ErrClosed) {
		t.Fatal("late connection was published", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-ex.Done():
	default:
		t.Fatal("late connection leaked")
	}
	if c.links[1] != nil {
		t.Fatal("closed facade retained the late connection")
	}
}

func TestMarketClientTimeoutChangedDuringDial(t *testing.T) {
	c := newMarketClient()
	ex := respondingTestClient(t, true, func(_, _ []byte) []byte { return make([]byte, 20) })
	started, release := make(chan struct{}), make(chan struct{})
	c.dial[1] = func() (*Conn, error) { close(started); <-release; return ex, nil }
	dialDone := make(chan error, 1)
	go func() { _, err := c.connection(protocol.ExchangeHK); dialDone <- err }()
	<-started
	var released sync.Once
	releaseDial := func() { released.Do(func() { close(release) }) }
	t.Cleanup(func() { releaseDial(); c.Close() })
	setDone := make(chan struct{})
	go func() { c.SetTimeout(3 * time.Second); close(setDone) }()
	select {
	case <-setDone:
	case <-time.After(time.Second):
		t.Fatal("SetTimeout blocked during dial")
	}
	releaseDial()
	if err := <-dialDone; err != nil {
		t.Fatal(err)
	}
	if ex.waitTimeout != 3*time.Second {
		t.Fatal("new connection lost the latest timeout", ex.waitTimeout)
	}
}
