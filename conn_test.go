package tdx

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestProtocolConnOwnership(t *testing.T) {
	standard := respondingTestClient(t, false, func(_, _ []byte) []byte { return nil })
	extended := respondingTestClient(t, true, func(_, _ []byte) []byte { return nil })
	std, ok := standard.protocolClient.(*StConn)
	if !ok || std.transport() != standard.conn || std.queries.source != std {
		t.Fatal("standard dial did not bind its business implementation")
	}
	ex, ok := extended.protocolClient.(*ExConn)
	if !ok || ex.transport() != extended.conn || ex.queries.source != ex {
		t.Fatal("extended dial did not bind its business implementation")
	}
	for _, core := range []*conn{standard.conn, extended.conn} {
		if _, ok := any(core).(Client); ok {
			t.Fatal("transport implements Client")
		}
	}
}

func TestNativeClientConcurrentWrites(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			c := respondingTestClient(t, ex, func(h, _ []byte) []byte {
				if ex {
					data := make([]byte, 23)
					binary.LittleEndian.PutUint32(data[19:23], binary.LittleEndian.Uint32(h[1:5]))
					return data
				}
				return protocol.Bytes(uint16(binary.LittleEndian.Uint32(h[1:5])))
			})
			var wg sync.WaitGroup
			for i := 0; i < 32; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					f := protocol.MCount.Frame(protocol.ExchangeSZ)
					if ex {
						f = protocol.MEx.FrameCount()
					}
					r, err := c.SendFrame(f)
					if err != nil {
						t.Error(err)
					} else if ex && r.(int) != int(f.MsgID) {
						t.Error("extended response associated with wrong request")
					} else if !ex && r.(*protocol.CountResp).Count != uint16(f.MsgID) {
						t.Error("response associated with wrong request")
					}
				}()
			}
			wg.Wait()
			if c.Info.WriteCount != 33 { // 握手 + 32 个请求。
				t.Fatalf("write count=%d, want 33", c.Info.WriteCount)
			}
		})
	}
}

func TestRequestTimeoutDuringWrite(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			c := pipeTestClient(t, ex, func(s net.Conn) {
				readTestFrame(s) // 握手后不再读取；等待连接关闭。
				var one [1]byte
				s.Read(one[:]) // 只读一字节，留下未发送完的帧。
				<-time.After(150 * time.Millisecond)
			})
			c.SetTimeout(20 * time.Millisecond)
			started := time.Now()
			f := protocol.MCount.Frame(protocol.ExchangeSZ)
			if ex {
				f = protocol.MEx.FrameCount()
			}
			_, err := c.SendFrame(f)
			if err == nil || err.Error() != "超时" || time.Since(started) > 120*time.Millisecond {
				t.Fatalf("write timeout took %s: %v", time.Since(started), err)
			}
			if _, err := c.SendFrame(f); err == nil || err.Error() == "超时" {
				t.Fatal("interrupted write did not close the connection", err)
			}
		})
	}
}

func TestRequestTimeoutWhileQueuedDoesNotWrite(t *testing.T) {
	var offsets []uint32
	c := respondingTestClient(t, false, func(h, _ []byte) []byte {
		offsets = append(offsets, binary.LittleEndian.Uint32(h[1:5]))
		return []byte{1, 0}
	})
	c.SetTimeout(20 * time.Millisecond)
	c.writeGate <- struct{}{}
	_, err := c.GetCount(protocol.ExchangeSZ)
	<-c.writeGate
	if err == nil || err.Error() != "超时" {
		t.Fatalf("expected queued timeout: %v", err)
	}
	c.SetTimeout(time.Second)
	if _, err := c.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal("queued timeout closed healthy connection", err)
	}
	if len(offsets) != 1 || offsets[0] != 2 {
		t.Fatal("expired request was sent", offsets)
	}
}

func TestTradeDecodeErrorReachesRequester(t *testing.T) {
	c := respondingTestClient(t, false, func(h, _ []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeKline {
			return completionStandardBars([]uint32{20250102}, nil, false)
		}
		return []byte{1, 0, 0, 0, 0, 0}
	})
	_, err := c.GetHistoryTrade("20250102", "sz000001", 0, 1)
	if err == nil || err.Error() == "超时" {
		t.Fatalf("expected decode error, got %v", err)
	}
}

func TestFrameDecodeErrorReachesRequester(t *testing.T) {
	c := pipeTestClient(t, false, func(s net.Conn) {
		if _, _, err := readTestFrame(s); err != nil {
			return
		}
		h, _, err := readTestFrame(s)
		if err != nil {
			return
		}
		r := make([]byte, 17)
		copy(r, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
		copy(r[5:9], h[1:5])
		copy(r[10:12], h[10:12])
		binary.LittleEndian.PutUint16(r[12:14], 1)
		binary.LittleEndian.PutUint16(r[14:16], 2)
		s.Write(r) // 数据不是合法 zlib 流。
	})
	f := protocol.MCount.Frame(protocol.ExchangeSZ)
	if _, err := c.SendFrame(f, "cached request"); err == nil || err.Error() == "超时" {
		t.Fatalf("expected frame decode error, got %v", err)
	}
	if c.Wait.IsWait("1") {
		t.Fatal("failed request callback leaked")
	}
	if _, ok := c.m.Get("1"); ok {
		t.Fatal("failed request cache leaked")
	}
}

func TestConcurrentWritesDuringReconnect(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			var dials atomic.Int32
			var servers sync.WaitGroup
			first := make(chan net.Conn, 1)
			reconnected := make(chan struct{}, 1)
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) {
				a, b := net.Pipe()
				n := dials.Add(1)
				if n == 1 {
					first <- b
				}
				servers.Add(1)
				go func() {
					defer servers.Done()
					defer b.Close()
					initialized := false
					for {
						h, _, err := readTestFrame(b)
						if err != nil {
							return
						}
						if !initialized {
							want := uint16(protocol.TypeConnect)
							if ex {
								want = protocol.TypeExSetup
							}
							if got := binary.LittleEndian.Uint16(h[10:12]); got != want {
								t.Errorf("business frame %04x preceded handshake %04x", got, want)
								return
							}
							initialized = true
							if n > 1 {
								reconnected <- struct{}{}
							}
						}
						if binary.LittleEndian.Uint32(h[1:5]) == 0 {
							continue
						}
						data := []byte{1, 0}
						if ex {
							data = make([]byte, 23)
							binary.LittleEndian.PutUint32(data[19:23], 1)
						}
						if err := writeTestResponse(b, h, data); err != nil {
							return
						}
					}
				}()
				return a, string(rune('a' + n)), nil
			}
			var c *testClient
			var err error
			if ex {
				c, err = testDialWith(dial, true, WithRedial(), WithDebug(false))
			} else {
				c, err = testDialWith(dial, false, WithRedial(), WithDebug(false))
			}
			if err != nil {
				t.Fatal(err)
			}
			c.SetTimeout(200 * time.Millisecond)
			stop := make(chan struct{})
			var once sync.Once
			var workers sync.WaitGroup
			t.Cleanup(func() {
				once.Do(func() { close(stop) })
				workers.Wait()
				c.Close()
				servers.Wait()
			})
			for i := 0; i < 8; i++ {
				workers.Add(1)
				go func(direct bool) {
					defer workers.Done()
					for {
						select {
						case <-stop:
							return
						default:
						}
						if direct {
							c.WriteString(string(protocol.MHeart.Frame().Bytes()))
						} else if ex {
							c.protocolClient.(*ExConn).getExtendedCount()
						} else {
							c.GetCount(protocol.ExchangeSZ)
						}
						time.Sleep(time.Millisecond)
					}
				}(i%2 == 0)
			}
			(<-first).Close()
			select {
			case <-reconnected:
			case <-time.After(3 * time.Second):
				t.Fatal("connection did not reconnect")
			}
			once.Do(func() { close(stop) })
			workers.Wait()
			c.SetTimeout(time.Second)
			if ex {
				if n, err := c.protocolClient.(*ExConn).getExtendedCount(); err != nil || n != 1 {
					t.Fatal("request failed after reconnect", n, err)
				}
			} else if r, err := c.GetCount(protocol.ExchangeSZ); err != nil || r != 1 {
				t.Fatal("request failed after reconnect", r, err)
			}
		})
	}
}

func TestCloseAllStopsReconnection(t *testing.T) {
	c := pipeTestClient(t, false, func(s net.Conn) {
		for {
			h, _, err := readTestFrame(s)
			if err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:5]) != 0 {
				if writeTestResponse(s, h, []byte{1, 0}) != nil {
					return
				}
			}
		}
	}, WithRedial(true))
	if _, err := c.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Runner2.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("read loop kept retrying after CloseAll")
	}
}

func TestNilDialReturnsError(t *testing.T) {
	for _, ex := range []bool{false, true} {
		if c, err := testDialWith(nil, ex); err == nil || c != nil {
			t.Fatal("nil dial must return an error", c, err)
		}
	}
}

type strictCloseConn struct {
	net.Conn
	closes atomic.Int32
}

func TestCloseAllRunsDisconnectCleanup(t *testing.T) {
	a, b := net.Pipe()
	transport := &strictCloseConn{Conn: a}
	serverDone := make(chan struct{})
	go func() { defer close(serverDone); io.Copy(io.Discard, b) }()
	var disconnected atomic.Int32
	c, err := DialWith(func(context.Context) (ios.ReadWriteCloser, string, error) {
		return transport, "strict-close", nil
	}, WithRedial(false), WithDebug(false), client.WithDisconnect(func(*client.Client, error) {
		disconnected.Add(1)
	}))
	if err != nil {
		b.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close(); b.Close(); <-serverDone })
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if transport.closes.Load() != 1 || disconnected.Load() != 1 {
		t.Fatalf("transport closes=%d disconnect callbacks=%d", transport.closes.Load(), disconnected.Load())
	}
	if err := c.Close(); err != nil || disconnected.Load() != 1 {
		t.Fatal("repeated close changed cleanup", err)
	}
}

func TestDialProtocolDefaults(t *testing.T) {
	for _, extended := range []bool{false, true} {
		name := "standard"
		if extended {
			name = "extended"
		}
		t.Run(name, func(t *testing.T) {
			a, b := net.Pipe()
			defer b.Close()
			frames := make(chan []byte, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				h, body, err := readTestFrame(b)
				if err != nil {
					return
				}
				frames <- append(h, body...)
				io.Copy(io.Discard, b)
			}()
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return a, "offline-init", nil }
			want, timeout := protocol.MConnect.Frame().Bytes(), 2*time.Second
			if extended {
				want, timeout = protocol.MEx.FrameSetup().Bytes(), 10*time.Second
			}
			c, err := testDialWith(dial, extended, WithRedial(false), WithLevel(LevelNone), WithDebug(false))
			if err != nil {
				a.Close()
				b.Close()
				<-done
				t.Fatal(err)
			}
			defer func() { c.Close(); <-done }()
			if got := <-frames; !bytes.Equal(got, want) {
				t.Fatalf("handshake changed: got %x, want %x", got, want)
			}
			if c.waitTimeout != timeout {
				t.Fatalf("timeout=%s, want %s", c.waitTimeout, timeout)
			}
		})
	}
}

type frameTestWriter func([]byte) (int, error)

func (w frameTestWriter) Write(data []byte) (int, error) { return w(data) }

func TestSendFrameLifecycle(t *testing.T) {
	for _, mode := range []string{"immediate", "timeout", "write failure", "short write"} {
		t.Run(mode, func(t *testing.T) {
			c := &conn{Client: client.New(nil), Wait: wait.New(time.Second), m: maps.NewSafe()}
			c.SetTimeout(10 * time.Millisecond)
			c.Client.MoreWriter = ios.NewMoreWrite(frameTestWriter(func(data []byte) (int, error) {
				if mode == "write failure" {
					return 0, io.ErrClosedPipe
				}
				if mode == "short write" {
					return len(data) - 1, nil
				}
				if mode == "immediate" {
					key := conv.String(binary.LittleEndian.Uint32(data[1:5]))
					c.waitMu.Lock()
					c.Wait.Done(key, "ok")
					c.waitMu.Unlock()
				}
				return len(data), nil
			}))
			c.wrapWriter(c.Client)
			f := &protocol.Frame{Type: protocol.TypeTrade}
			value, err := c.SendFrame(f, "cached request")
			switch mode {
			case "immediate":
				if err != nil || value != "ok" {
					t.Fatal(value, err)
				}
			case "timeout":
				if err == nil || err.Error() != "超时" {
					t.Fatalf("expected timeout, got %v", err)
				}
			case "write failure":
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("expected write failure, got %v", err)
				}
			case "short write":
				if !errors.Is(err, io.ErrShortWrite) {
					t.Fatalf("expected short write, got %v", err)
				}
			}
			key := conv.String(f.MsgID)
			if c.Wait.IsWait(key) {
				t.Fatal("request callback leaked")
			}
			if _, ok := c.m.Get(key); ok {
				t.Fatal("request cache leaked")
			}
		})
	}
}

func TestSendFrameConcurrentImmediateResponses(t *testing.T) {
	c := &conn{Client: client.New(nil), Wait: wait.New(time.Second), m: maps.NewSafe()}
	c.SetTimeout(time.Second)
	writer := ios.NewMoreWrite(frameTestWriter(func(data []byte) (int, error) {
		id := binary.LittleEndian.Uint32(data[1:5])
		c.waitMu.Lock()
		c.Wait.Done(conv.String(id), id)
		c.waitMu.Unlock()
		return len(data), nil
	}))
	writer.OnWrite = func(write func() error) error { return write() }
	c.Client.MoreWriter = writer
	c.wrapWriter(c.Client)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := &protocol.Frame{Type: protocol.TypeCount}
			value, err := c.SendFrame(f)
			if err != nil || value != f.MsgID {
				t.Errorf("response=%v, message ID=%d, error=%v", value, f.MsgID, err)
			}
		}()
	}
	wg.Wait()
}

func TestSendFrameTimeoutWithBlockedWriter(t *testing.T) {
	c := &conn{Client: client.New(nil), Wait: wait.New(time.Second), m: maps.NewSafe()}
	c.SetTimeout(20 * time.Millisecond)
	release := make(chan struct{})
	started := make(chan struct{})
	finished := make(chan struct{})
	c.Client.MoreWriter = ios.NewMoreWrite(frameTestWriter(func(bs []byte) (int, error) {
		close(started)
		<-release
		defer close(finished)
		return len(bs), nil
	}))
	c.wrapWriter(c.Client)
	t.Cleanup(func() {
		close(release)
		<-finished
	})
	f := protocol.MCount.Frame(protocol.ExchangeSZ)
	done := make(chan error, 1)
	go func() { _, err := c.SendFrame(f, "request cache"); done <- err }()
	<-started
	select {
	case err := <-done:
		if err == nil || err.Error() != "超时" {
			t.Fatal("expected timeout", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("blocked writer prevented timeout")
	}
	key := conv.String(f.MsgID)
	if c.Wait.IsWait(key) {
		t.Fatal("timed-out request callback leaked")
	}
	if _, ok := c.m.Get(key); ok {
		t.Fatal("timed-out request cache leaked")
	}
}

func TestHandshakeBeforeConcurrentWrites(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			var dials, setups atomic.Int32
			var servers sync.WaitGroup
			first := make(chan net.Conn, 1)
			frames := make(chan uint16, 8)
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			setup := func() *protocol.Frame {
				if setups.Add(1) == 2 {
					close(started)
					<-release
				}
				if ex {
					return protocol.MEx.FrameSetup()
				}
				return protocol.MConnect.Frame()
			}
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) {
				a, b := net.Pipe()
				n := dials.Add(1)
				if n == 1 {
					first <- b
				}
				servers.Add(1)
				go func() {
					defer servers.Done()
					defer b.Close()
					for {
						h, _, err := readTestFrame(b)
						if err != nil {
							return
						}
						if n > 1 {
							frames <- binary.LittleEndian.Uint16(h[10:12])
						}
					}
				}()
				return a, "pipe", nil
			}
			c, err := dialWith(context.Background(), dial, time.Second, setup, protocol.MHeart.Frame, func(*protocol.Response, any) (any, error) { return nil, nil }, WithRedial(), WithDebug(false))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { once.Do(func() { close(release) }); c.Close(); servers.Wait() })
			(<-first).Close()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("reconnect did not reach handshake")
			}
			written := make(chan error, 1)
			go func() {
				_, err := c.Write((&protocol.Frame{MsgID: 1, Type: protocol.TypeCount}).Bytes())
				written <- err
			}()
			select {
			case typ := <-frames:
				t.Fatalf("frame %04x escaped before handshake", typ)
			case <-time.After(50 * time.Millisecond):
			}
			once.Do(func() { close(release) })
			want := uint16(protocol.TypeConnect)
			if ex {
				want = protocol.TypeExSetup
			}
			for _, typ := range []uint16{want, protocol.TypeCount} {
				select {
				case got := <-frames:
					if got != typ {
						t.Fatalf("frame=%04x want=%04x", got, typ)
					}
				case <-time.After(time.Second):
					t.Fatal("missing frame")
				}
			}
			if err := <-written; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMalformedResponsesReachCaller(t *testing.T) {
	c := respondingTestClient(t, false, func(h, _ []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeQuote {
			return []byte{0, 0, 1, 0}
		}
		return []byte{1, 0}
	})
	for _, f := range []*protocol.Frame{protocol.MCode.Frame(protocol.ExchangeSH, 0), {Control: protocol.Control01, Type: protocol.TypeKline, Data: make([]byte, 28)}, {Control: protocol.Control01, Type: protocol.TypeQuote}} {
		start := time.Now()
		if _, err := c.SendFrame(f, protocol.KlineCache{Type: protocol.TypeKlineDay}); err == nil || time.Since(start) >= 500*time.Millisecond {
			t.Fatal("malformed response was swallowed until timeout", err)
		}
	}
}
