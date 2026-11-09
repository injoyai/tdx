package tdx

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestContextCancellationKeepsSharedTransport(t *testing.T) {
	received, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	c := pipeTestClient(t, false, func(s net.Conn) {
		for {
			h, _, err := readTestFrame(s)
			if err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:5]) == 0 {
				continue
			}
			once.Do(func() { close(received); <-release })
			if err := writeTestResponse(s, h, testStandardBar(false)); err != nil {
				return
			}
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var unblock sync.Once
	defer unblock.Do(func() { close(release) })
	bound := WithContext(ctx, c.protocolClient)
	done := make(chan error, 1)
	go func() { _, err := bound.GetKlineDay("sz000001", 0, 1); done <- err }()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	// Seeing bytes on the server alone can race with the sender finishing Write.
	// Acquire the writer gate to prove cancellation happens while awaiting a response.
	c.conn.writer.gate <- struct{}{}
	<-c.conn.writer.gate
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("request ignored cancellation")
	}
	unblock.Do(func() { close(release) })
	r, err := c.GetKlineDay("sz000001", 0, 1)
	if err != nil || r.Count != 1 {
		t.Fatal("shared transport was poisoned", r, err)
	}
}

func TestQuoteContextCancelsWhileAnotherQuoteIsActive(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var serveOnce, releaseOnce sync.Once
	c := respondingTestClient(t, false, func(_, body []byte) []byte {
		serveOnce.Do(func() { close(entered); <-release })
		return testQuoteResponse(body)
	})
	owner := make(chan error, 1)
	go func() { _, err := c.GetQuote("sz000001"); owner <- err }()
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		select {
		case err := <-owner:
			if err != nil {
				t.Error("active quote was affected", err)
			}
		case <-time.After(time.Second):
			t.Error("active quote did not finish")
		}
	})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("active quote did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &observedContext{Context: ctx, waiting: make(chan struct{})}
	done := make(chan error, 1)
	go func() { _, err := WithContext(observed, c.protocolClient).GetQuote("sz000001"); done <- err }()
	select {
	case <-observed.waiting:
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("canceled quote waits for another query")
		releaseOnce.Do(func() { close(release) })
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("waiting quote did not finish")
		}
	}
}

func TestContextStopsPagination(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pages atomic.Int32
	c := respondingTestClient(t, true, func(_, _ []byte) []byte {
		pages.Add(1)
		data := make([]byte, 20+800*32)
		binary.LittleEndian.PutUint16(data[18:20], 800)
		for i := 0; i < 800; i++ {
			copy(data[20+i*32:], testExtendedBar(20261009)[20:])
		}
		return data
	})
	bound := WithContext(ctx, c.protocolClient)
	_, err := bound.GetKlineUntil(protocol.TypeKlineDay, "hk00700", func(Kline) bool { cancel(); return false })
	if !errors.Is(err, context.Canceled) || pages.Load() != 1 {
		t.Fatal("continued canceled pagination", pages.Load(), err)
	}
}

func TestInitialDialContextDoesNotBecomeConnectionLifetime(t *testing.T) {
	clientEnd, serverEnd := net.Pipe()
	defer serverEnd.Close()
	go io.Copy(io.Discard, serverEnd)
	ctx, cancel := context.WithCancel(context.Background())
	c, err := dialStContexts(context.Background(), ctx, func(context.Context) (ios.ReadWriteCloser, string, error) { return clientEnd, "offline-test", nil }, WithRedial(false), WithLevel(LevelNone))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer c.Close()
	cancel()
	if c.Closed() {
		t.Fatal("request canceled shared lifetime")
	}
	// A subsequent write uses the connection after both initial dial contexts ended.
	if _, err := c.output.Write(protocol.MHeart.Frame().Bytes()); err != nil {
		t.Fatal(err)
	}
}

type observedContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestContextCancelsInitialDialAndGateWait(t *testing.T) {
	c := newMarketClient()
	defer c.Close()
	entered := make(chan struct{})
	c.dial[0] = func(ctx context.Context) (protocolClient, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := WithContext(ctx, c).GetKlineDay("sz000001", 0, 1); done <- err }()
	<-entered
	queued, stop := context.WithCancel(context.Background())
	defer stop()
	observed := &observedContext{Context: queued, waiting: make(chan struct{})}
	queuedDone := make(chan error, 1)
	go func() { _, err := WithContext(observed, c).GetKlineDay("sz000001", 0, 1); queuedDone <- err }()
	select {
	case <-observed.waiting:
	case <-time.After(time.Second):
		t.Fatal("did not wait for initial connection")
	}
	stop()
	select {
	case err := <-queuedDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled query waited for another dial")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("dial ignored cancellation")
	}
	if c.closed {
		t.Fatal("canceled query closed client")
	}
}
