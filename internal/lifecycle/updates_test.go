package lifecycle

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMetadataCloseWaitsForRefresh(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var cleanup atomic.Int32
	u := &Updates{}
	closeUpdates := func() error { return u.Close(nil, func() error { cleanup.Add(1); return nil }) }
	defer func() { unblock(); closeUpdates() }()
	refreshed := make(chan error, 1)
	go func() { refreshed <- u.Run(func() error { close(entered); <-release; return nil }) }()
	<-entered
	closed := make(chan struct{})
	go func() { closeUpdates(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("closed resources during refresh")
	case <-time.After(20 * time.Millisecond):
	}
	if cleanup.Load() != 0 {
		t.Fatal("cleanup ran during refresh")
	}
	unblock()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if cleanup.Load() != 1 {
		t.Fatal("cleanup count", cleanup.Load())
	}
	if err := u.Run(func() error { t.Error("started refresh after close"); return nil }); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}
