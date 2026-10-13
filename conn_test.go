package tdx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
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

func TestProtocolClientQueries(t *testing.T) {
	for _, extended := range []bool{false, true} {
		name, code, wrong := "standard", "sz000001", "hk00700"
		if extended {
			name, code, wrong = "extended", wrong, code
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			var daily, history atomic.Int32
			native := respondingTestClient(t, extended, func(h, b []byte) []byte {
				requests.Add(1)
				typ := binary.LittleEndian.Uint16(h[10:])
				if h[0] != 0x0c && !extended || h[0] != 0x01 && extended {
					t.Errorf("wrong request prefix %x", h[0])
				}
				switch typ {
				case protocol.TypeQuote:
					return testQuoteResponse(b)
				case protocol.TypeExQuote:
					data := make([]byte, 150)
					copy(data, b)
					return data
				case protocol.TypeKline:
					daily.Add(1)
					return testStandardBar(false)
				case protocol.TypeExBars:
					daily.Add(1)
					return testExtendedBar(20261009)
				case protocol.TypeHistoryTrade:
					history.Add(1)
					return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 1, 0}
				case protocol.TypeExHistTrade:
					history.Add(1)
					data := make([]byte, 32)
					binary.LittleEndian.PutUint16(data[14:], 1)
					return data
				default:
					t.Errorf("unexpected request type %x", typ)
					return nil
				}
			})
			var api Client = native.protocolClient
			quotes, err := api.GetQuote(code, code)
			if err != nil || len(quotes) != 2 || quotes[0].Instrument != quotes[1].Instrument {
				t.Fatal("quote order/duplicates changed", quotes, err)
			}
			bars, err := api.GetKlineDayAll(code)
			if err != nil || bars.Count != 1 {
				t.Fatal("shared pagination failed", bars, err)
			}
			before := daily.Load()
			trades, err := api.GetHistoryTradeDay("20261009", code)
			if err != nil || trades.Count != 1 || trades.List[0].Date != "20261009" || daily.Load()-before != 1 || history.Load() != 1 {
				t.Fatal("history check/date changed", trades, err, daily.Load(), history.Load())
			}
			beforeRequests := requests.Load()
			for _, call := range []func() error{
				func() error { _, err := api.GetQuote(wrong); return err },
				func() error { _, err := api.GetKlineDayAll(wrong); return err },
				func() error { _, err := api.GetHistoryMinute("20261009", wrong); return err },
				func() error { _, err := api.GetHistoryTrade("20261009", wrong, 0, 1); return err },
				func() error { _, err := api.GetKlineRange(wrong, "20261009", "20261009"); return err },
			} {
				if err := call(); err == nil {
					t.Fatal("other protocol accepted")
				}
			}
			if extended {
				if _, err := api.GetFinanceInfo(protocol.ExchangeSZ, "000001"); err == nil {
					t.Fatal("extended client accepted finance")
				}
				if _, err := api.GetReportFile("a"); err == nil {
					t.Fatal("extended client accepted standard report")
				}
			}
			if requests.Load() != beforeRequests {
				t.Fatal("unsupported query sent a packet")
			}
			if err := api.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := api.GetMarkets(); !errors.Is(err, net.ErrClosed) {
				t.Fatal("closed implementation remained usable", err)
			}
		})
	}
}

func TestClientUnifiedBjCodes(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create(protocol.FileTdxBjMore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write([]byte("44|920000|2|First|1|\n44|920001|2|Second|1|\n")); err != nil {
		t.Fatal(err)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) != protocol.TypeBlockInfo || binary.LittleEndian.Uint32(b) != 0 {
			t.Error("BJ codes did not use report source")
		}
		return append(protocol.Bytes(uint32(buf.Len())), buf.Bytes()...)
	})
	if n, err := c.GetCount(protocol.ExchangeBJ); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	page, err := c.GetCode(protocol.ExchangeBJ, 1, 1)
	if err != nil || page.Count != 1 || page.List[0].Instrument.Code != "920001" || page.List[0].Standard.Name != "Second" {
		t.Fatal(page, err)
	}
}

func testCodePage(start uint16) []byte {
	codes := []string{"000001", "000002", "000003"}
	if int(start) >= len(codes) {
		return []byte{0, 0}
	}
	codes = codes[start:]
	b := make([]byte, 2+len(codes)*29)
	binary.LittleEndian.PutUint16(b, uint16(len(codes)))
	for j, code := range codes {
		p := 2 + j*29
		copy(b[p:], code)
		binary.LittleEndian.PutUint16(b[p+6:], 100)
		copy(b[p+8:], "NAME")
		b[p+20] = 4
	}
	return b
}

func testInstrumentPage(start uint32, count uint16) []byte {
	items := []protocol.ExInstrument{{Market: 31, Code: "00700", Name: "Tencent", Category: 2}, {Market: 74, Code: "AAPL", Name: "Apple", Category: 13}, {Market: 31, Code: "00005", Name: "HSBC", Category: 2}}
	if uint64(start) >= uint64(len(items)) {
		return make([]byte, 6)
	}
	items = items[int(start):min(int(start)+int(count), len(items))]
	b := make([]byte, 6+64*len(items))
	binary.LittleEndian.PutUint16(b[4:], uint16(len(items)))
	for j, it := range items {
		p := 6 + j*64
		b[p], b[p+1] = it.Category, it.Market
		copy(b[p+5:], it.Code)
		copy(b[p+14:], it.Name)
		copy(b[p+31:], "desc")
	}
	return b
}

func testMarketTable() []byte {
	b := make([]byte, 2+64)
	binary.LittleEndian.PutUint16(b, 1)
	b[2] = 13
	copy(b[3:], "Other")
	b[35] = 200 // 发现未接入的市场时仍保留原始编号。
	copy(b[36:], "OT")
	return b
}

func TestClientUnifiedCodes(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			var sent atomic.Int32
			c := respondingTestClient(t, ex, func(h, b []byte) []byte {
				sent.Add(1)
				switch binary.LittleEndian.Uint16(h[10:]) {
				case protocol.TypeCount:
					return []byte{3, 0}
				case protocol.TypeCode:
					return testCodePage(binary.LittleEndian.Uint16(b[2:]))
				case protocol.TypeExCount:
					r := make([]byte, 23)
					binary.LittleEndian.PutUint32(r[19:], 3)
					return r
				case protocol.TypeExInstrument:
					return testInstrumentPage(binary.LittleEndian.Uint32(b), binary.LittleEndian.Uint16(b[4:]))
				default:
					t.Error("unexpected request", h)
					return nil
				}
			})
			exchange, wrong, want := protocol.ExchangeSZ, protocol.ExchangeHK, 3
			if ex {
				exchange, wrong, want = wrong, exchange, 2
			}
			n, err := c.GetCount(exchange)
			if err != nil || n != want {
				t.Fatal(n, err)
			}
			all, err := c.GetCodeAll(exchange)
			if err != nil || all.Count != want || len(all.List) != want {
				t.Fatal(all, err)
			}
			page, err := c.GetCode(exchange, 1, 1)
			if err != nil || page.Count != 1 || len(page.List) != 1 {
				t.Fatal(page, err)
			}
			item := page.List[0]
			if item.Instrument.Exchange != exchange {
				t.Fatal(item)
			}
			if ex {
				if item.Instrument.Code != "00005" || item.Extended.Category != 2 || item.Extended.Desc != "desc" {
					t.Fatal("global offset was used as market offset", item)
				}
				page, err = c.GetCode(exchange, ^uint32(0), 1)
			} else {
				if item.Instrument.Code != "000002" || item.Standard.Multiple != 100 || item.Standard.Decimal != 4 {
					t.Fatal("standard metadata lost", item)
				}
				page, err = c.GetCode(exchange, 3, 1)
			}
			if err != nil || page.Count != 0 || len(page.List) != 0 {
				t.Fatal(page, err)
			}
			before := sent.Load()
			for _, call := range []func() error{
				func() error { _, e := c.GetCode(exchange, 0, 0); return e },
				func() error { _, e := c.GetCode(exchange, 0, 1001); return e },
				func() error { _, e := c.GetCode(wrong, 0, 1); return e },
				func() error { _, e := c.GetCount(wrong); return e },
				func() error { _, e := c.GetCodeAll(wrong); return e },
				func() error { _, e := c.GetCodeAll(255); return e },
			} {
				if e := call(); e == nil {
					t.Fatal("invalid query accepted")
				}
			}
			if !ex {
				if _, e := c.GetCode(exchange, 65536, 1); e == nil {
					t.Fatal("offset overflow accepted")
				}
			}
			if sent.Load() != before {
				t.Fatal("invalid query sent a request")
			}
		})
	}
}

// testClient lets protocol-independent transport tests inspect the shared core.
type testClient struct {
	protocolClient
	*conn
}

func (c *testClient) Close() error { return c.protocolClient.Close() }

func (c *testClient) SetTimeout(t time.Duration) { c.protocolClient.SetTimeout(t) }

func (c *testClient) Done() <-chan struct{} { return c.conn.Done() }

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

func TestClientUnifiedQueries(t *testing.T) {
	for _, extended := range []bool{false, true} {
		name, code, wrong := "standard", "sz000001", "hk00700"
		if extended {
			name, code, wrong = "extended", wrong, code
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			c := respondingTestClient(t, extended, func(h, b []byte) []byte {
				requests.Add(1)
				typ := binary.LittleEndian.Uint16(h[10:12])
				switch typ {
				case protocol.TypeQuote:
					return testQuoteResponse(b)
				case protocol.TypeExQuote:
					data := make([]byte, 150)
					copy(data, b)
					binary.LittleEndian.PutUint32(data[30:34], math.Float32bits(10))
					return data
				case protocol.TypeKline:
					return testStandardBar(false)
				case protocol.TypeExBars:
					return testExtendedBar(20261009)
				case protocol.TypeHistoryMinute:
					return []byte{1, 0, 0, 0, 0, 0, 0xa8, 0x0f, 0, 2}
				case protocol.TypeExMinute, protocol.TypeExHistMinute:
					header := 12
					if typ == protocol.TypeExHistMinute {
						header = 20
					}
					data := make([]byte, header+18)
					binary.LittleEndian.PutUint16(data[header-2:header], 1)
					binary.LittleEndian.PutUint16(data[header:header+2], 570)
					binary.LittleEndian.PutUint32(data[header+2:header+6], math.Float32bits(10))
					return data
				case protocol.TypeTrade:
					return []byte{1, 0, 0x3a, 2, 0xa8, 0x0f, 1, 3, 1, 0}
				case protocol.TypeHistoryTrade:
					return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 1, 0}
				case protocol.TypeExTrade, protocol.TypeExHistTrade:
					data := make([]byte, 32)
					binary.LittleEndian.PutUint16(data[14:16], 1)
					binary.LittleEndian.PutUint16(data[16:18], 570)
					binary.LittleEndian.PutUint32(data[18:22], 10000)
					binary.LittleEndian.PutUint16(data[30:32], 256)
					return data
				case protocol.TypeExQuoteList:
					return []byte{0, 0}
				default:
					t.Errorf("unexpected request type %x", typ)
					return nil
				}
			})
			quotes, err := c.GetQuote(code)
			if err != nil || len(quotes) != 1 || quotes[0].Price != 10 {
				t.Fatal(quotes, err)
			}
			bars, err := c.GetKline(protocol.TypeKlineDay, code, 0, 1)
			if err != nil || bars.Count != 1 {
				t.Fatal(bars, err)
			}
			minute, err := c.GetMinute(code)
			if err != nil || minute.Count != 1 || minute.List[0].Price != 10 {
				t.Fatal(minute, err)
			}
			minute, err = c.GetHistoryMinute("20261009", code)
			if err != nil || minute.Count != 1 || minute.Date != "20261009" {
				t.Fatal(minute, err)
			}
			trade, err := c.GetTrade(code, 0, 1)
			if err != nil || trade.Count != 1 || trade.List[0].Direction != -1 {
				t.Fatal(trade, err)
			}
			trade, err = c.GetHistoryTrade("20261009", code, 0, 1)
			if err != nil || trade.Count != 1 || trade.Date != "20261009" {
				t.Fatal(trade, err)
			}
			if extended {
				if quotes[0].Extended == nil || bars.List[0].Extended == nil || minute.List[0].Extended == nil || trade.List[0].Extended == nil || trade.List[0].Price != nil {
					t.Fatal("lost extended data or guessed trade scale")
				}
			} else if quotes[0].Standard == nil || bars.List[0].Standard == nil || minute.List[0].Standard == nil || trade.List[0].Standard == nil || trade.List[0].Price == nil || *trade.List[0].Price != 10 {
				t.Fatal("lost standard data")
			}
			before := requests.Load()
			for _, call := range []func() error{
				func() error { _, err := c.GetQuote(wrong); return err },
				func() error { _, err := c.GetKline(9, wrong, 0, 1); return err },
				func() error { _, err := c.GetMinute(wrong); return err },
				func() error { _, err := c.GetTrade(wrong, 0, 1); return err },
			} {
				if err := call(); err == nil {
					t.Fatal("single connection accepted the other protocol")
				}
			}
			if requests.Load() != before {
				t.Fatal("protocol mismatch sent a request")
			}
			if _, ok := any(c.transport()).(Client); ok {
				t.Fatal("transport still implements the business interface")
			}

			list, err := c.GetQuoteList(31, 2, 0, 1)
			if extended && (err != nil || len(list) != 0) {
				t.Fatal("GetQuoteList migration failed", list, err)
			}
			if !extended && err == nil {
				t.Fatal("standard connection accepted an extended quote list")
			}
		})
	}
}
