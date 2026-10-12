package tdx

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// testClient lets protocol-independent transport tests inspect the shared core.
type testClient struct {
	protocolClient
	*conn
}

func (c *testClient) Close() error               { return c.protocolClient.Close() }
func (c *testClient) SetTimeout(t time.Duration) { c.protocolClient.SetTimeout(t) }
func (c *testClient) Done() <-chan struct{}      { return c.conn.Done() }

func testDialWith(dial ios.DialFunc, ex bool, options ...client.Option) (*testClient, error) {
	var c protocolClient
	var err error
	if ex {
		c, err = dialExHqWith(dial, options...)
	} else {
		c, err = DialWith(dial, options...)
	}
	if err != nil {
		return nil, err
	}
	core := c.(interface{ transport() *conn }).transport()
	return &testClient{protocolClient: c, conn: core}, nil
}

func readTestFrame(r io.Reader) ([]byte, []byte, error) {
	h := make([]byte, 12)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, nil, err
	}
	n := int(binary.LittleEndian.Uint16(h[6:8])) - 2
	if n < 0 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return h, b, err
}

func writeTestResponse(w io.Writer, h, data []byte) error {
	r := make([]byte, 16+len(data))
	copy(r, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
	copy(r[5:9], h[1:5])
	copy(r[10:12], h[10:12])
	binary.LittleEndian.PutUint16(r[12:14], uint16(len(data)))
	binary.LittleEndian.PutUint16(r[14:16], uint16(len(data)))
	copy(r[16:], data)
	_, err := w.Write(r)
	return err
}

func pipeTestClient(t *testing.T, ex bool, serve func(net.Conn), options ...client.Option) *testClient {
	t.Helper()
	a, b := net.Pipe()
	serverDone := make(chan struct{})
	go func() { defer close(serverDone); serve(b) }()
	dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return a, "offline-test", nil }
	var c *testClient
	var err error
	op := append([]client.Option{WithRedial(false), WithLevel(LevelNone)}, options...)
	if ex {
		c, err = testDialWith(dial, true, op...)
	} else {
		c, err = testDialWith(dial, false, op...)
	}
	if err != nil {
		a.Close()
		b.Close()
		t.Fatal(err)
	}
	c.Logger.Debug(false)
	c.SetTimeout(time.Second)
	t.Cleanup(func() {
		c.Close()
		b.Close()
		<-serverDone
	})
	return c
}

func respondingTestClient(t *testing.T, ex bool, data func([]byte, []byte) []byte) *testClient {
	return pipeTestClient(t, ex, func(s net.Conn) {
		for {
			h, b, err := readTestFrame(s)
			if err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:5]) == 0 {
				continue // 握手与心跳没有等待者。
			}
			if err := writeTestResponse(s, h, data(h, b)); err != nil {
				return
			}
		}
	})
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

func TestBlockDownloadIntegrity(t *testing.T) {
	for _, mode := range []string{"complete", "empty", "truncated", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			c := respondingTestClient(t, false, func(h, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeBlockMeta {
					return protocol.Bytes(uint32(16))
				}
				start := binary.LittleEndian.Uint32(b[:4])
				if mode == "empty" || (mode == "truncated" && start > 0) {
					return []byte{0, 0, 0, 0}
				}
				if mode == "oversized" {
					return make([]byte, 4+17)
				}
				return []byte{4, 0, 0, 0, 'a', 'b', 'c', 'd'}
			})
			bs, err := c.GetBlockFileRaw(protocol.BlockFileGN)
			if mode == "complete" {
				if err != nil || string(bs) != "abcdabcdabcdabcd" {
					t.Fatal(string(bs), err)
				}
			} else if err == nil || bs != nil {
				t.Fatal("incomplete download reported success", len(bs), err)
			} else if mode != "oversized" && !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("expected unexpected EOF", err)
			}
		})
	}
}

func TestTradePaginationOverflow(t *testing.T) {
	for _, historical := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "historical"}[historical], func(t *testing.T) {
			var previous uint16
			c := respondingTestClient(t, false, func(h []byte, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeKline {
					return completionStandardBars([]uint32{20250102}, nil, false)
				}
				pos := 8
				if historical {
					pos = 12
				}
				start := binary.LittleEndian.Uint16(b[pos : pos+2])
				count := binary.LittleEndian.Uint16(b[pos+2 : pos+4])
				if start < previous {
					t.Error("pagination wrapped", previous, start)
				}
				previous = start
				out := protocol.Bytes(count)
				if historical {
					out = append(out, 0, 0, 0, 0)
				}
				for i := uint16(0); i < count; i++ {
					out = append(out, 0x3a, 2, 0xa8, 0x0f, 1, 0, 0)
					if !historical {
						out = append(out, 0)
					}
				}
				return out
			})
			var r *TradeResp
			var err error
			if historical {
				r, err = c.GetHistoryTradeDay("20250102", "sz000001")
			} else {
				r, err = c.GetTradeAll("sz000001")
			}
			if err == nil || !strings.Contains(err.Error(), "uint16") || r != nil {
				t.Fatal("expected pagination limit error", r, err)
			}
		})
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
				} else {
					select {
					case reconnected <- struct{}{}:
					default:
					}
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

func TestOtherPaginationLimits(t *testing.T) {
	for _, mode := range []string{"code", "stock", "index", "until"} {
		t.Run(mode, func(t *testing.T) {
			c := respondingTestClient(t, false, func(h, _ []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeCode {
					return append(protocol.Bytes(uint16(1000)), make([]byte, 1000*29)...)
				}
				record := make([]byte, 16)
				binary.LittleEndian.PutUint32(record, 20250102)
				if mode == "index" {
					record = append(record, 0, 0, 0, 0)
				}
				out := protocol.Bytes(uint16(800))
				for i := 0; i < 800; i++ {
					out = append(out, record...)
				}
				return out
			})
			var err error
			switch mode {
			case "code":
				_, err = c.GetCodeAll(protocol.ExchangeSZ)
			case "index":
				_, err = c.GetKlineAll(protocol.TypeKlineDay, "sh000001")
			case "stock":
				_, err = c.GetKlineAll(protocol.TypeKlineDay, "sz000001")
			case "until":
				seen := 0
				r, e := c.GetKlineUntil(protocol.TypeKlineDay, "sz000001", func(Kline) bool {
					seen++
					return seen == 65535
				})
				if e != nil || r.Count != 65535 || len(r.List) != 65535 {
					t.Fatal("valid boundary stop failed", e)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "uint16") {
				t.Fatal("expected pagination limit error", err)
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

func (c *strictCloseConn) Close() error {
	if c.closes.Add(1) > 1 {
		return net.ErrClosed
	}
	return c.Conn.Close()
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
