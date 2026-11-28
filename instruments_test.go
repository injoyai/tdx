package tdx

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestInstrumentsGlobalPaginationAndCompleteness(t *testing.T) {
	for _, mode := range []string{"complete", "empty", "missing", "first-missing", "early-short", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			pages := 0
			c := marketTestClient(t, nil, func(h, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeExCount {
					data := make([]byte, 23)
					if mode != "empty" {
						binary.LittleEndian.PutUint32(data[19:23], 801)
					}
					return data
				}
				pages++
				start, n := binary.LittleEndian.Uint32(b[:4]), int(binary.LittleEndian.Uint16(b[4:6]))
				wantStart, wantCount := uint32(800), 1
				if mode == "early-short" {
					wantStart, wantCount = 799, 2
				}
				if (pages == 1 && (start != 0 || n != 800)) || (pages == 2 && (start != wantStart || n != wantCount)) || pages > 2 {
					t.Error("incorrect global pagination", start, n)
				}
				if mode == "early-short" && start == 0 {
					n--
				} else if mode == "first-missing" || (mode == "missing" || mode == "early-short") && start != 0 {
					n = 0
				}
				if mode == "oversized" {
					n++
				}
				return instrumentPage(n, protocol.ExchangeUS.Uint8(), "AAPL")
			})
			rows, err := c.GetInstruments()
			if mode == "missing" || mode == "first-missing" || mode == "early-short" || mode == "oversized" {
				if !errors.Is(err, io.ErrUnexpectedEOF) || rows != nil {
					t.Fatal("incomplete discovery returned partial success", len(rows), err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "empty" {
				if rows == nil || len(rows) != 0 || pages != 0 {
					t.Fatal("empty discovery queried a page", len(rows), pages)
				}
			} else if len(rows) != 801 || rows[800].Code != "AAPL" || pages != 2 {
				t.Fatal("global discovery lost instruments", len(rows), pages)
			}
		})
	}
	if _, err := new(StConn).GetInstruments(); err == nil {
		t.Fatal("standard connection accepted extended discovery")
	}
}

func instrumentPage(n int, market uint8, code string) []byte {
	data := make([]byte, 6+n*64)
	binary.LittleEndian.PutUint16(data[4:6], uint16(n))
	for j := 0; j < n; j++ {
		p := 6 + j*64
		data[p+1] = market
		copy(data[p+5:p+14], code)
	}
	return data
}

func TestInstrumentsRejectConnectionChange(t *testing.T) {
	for _, position := range []string{"count", "last page"} {
		t.Run(position, func(t *testing.T) {
			var c *testClient
			changeWriter := func() {
				c.waitMu.Lock()
				next := *c.writer
				next.server = "replacement connection"
				c.writer = &next
				c.waitMu.Unlock()
			}
			c = respondingTestClient(t, true, func(h, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeExCount {
					if position == "count" {
						changeWriter()
					}
					data := make([]byte, 23)
					binary.LittleEndian.PutUint32(data[19:23], 801)
					return data
				}
				n := int(binary.LittleEndian.Uint16(b[4:6]))
				if position == "last page" && binary.LittleEndian.Uint32(b[:4]) == 800 {
					changeWriter()
				}
				return instrumentPage(n, protocol.ExchangeUS.Uint8(), "AAPL")
			})
			rows, err := c.GetInstruments()
			if !errors.Is(err, io.ErrUnexpectedEOF) || rows != nil {
				t.Fatalf("changed connection returned %d rows, err=%v", len(rows), err)
			}
		})
	}
}

func TestInstrumentsReportedTail(t *testing.T) {
	const total, lastPage, end = 110910, 110400, 110893
	for _, tc := range []struct {
		mode   string
		checks int
	}{
		{"confirmed", 4},
		{"short-continues", 0},
		{"last-changed", 1},
		{"no-longer-empty", 2},
		{"later-data", 3},
		{"count-changed", 4},
		{"decode-error", 2},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			mode := tc.mode
			var log bytes.Buffer
			counts, checks := 0, 0
			c := respondingTestClient(t, true, func(h, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeExCount {
					data := make([]byte, 23)
					n := uint32(total)
					counts++
					if counts > 1 {
						checks++
						if mode == "count-changed" {
							n = end
						}
					}
					binary.LittleEndian.PutUint32(data[19:23], n)
					return data
				}
				start, n := int(binary.LittleEndian.Uint32(b[:4])), int(binary.LittleEndian.Uint16(b[4:6]))
				switch {
				case start < lastPage && n == 800:
					return instrumentPage(n, protocol.ExchangeUS.Uint8(), "NVDA")
				case start == lastPage && n == total-lastPage:
					return instrumentPage(end-lastPage, 105, "SCH01805")
				case start == end && n == total-end:
					if mode == "short-continues" {
						return instrumentPage(n, 105, "SCH01806")
					}
				case start == end-1 && n == 1:
					checks++
					if mode == "last-changed" {
						return instrumentPage(1, 105, "SCH01806")
					}
					return instrumentPage(1, 105, "SCH01805")
				case start == end && n == 800:
					checks++
					if mode == "no-longer-empty" {
						return instrumentPage(1, 105, "SCH01806")
					}
					if mode == "decode-error" {
						return []byte{0} // Malformed page, distinct from a decoded empty page.
					}
				case start == total-1 && n == 1:
					checks++
					if mode == "later-data" {
						return instrumentPage(1, 105, "SCH01807")
					}
				default:
					t.Errorf("unexpected instrument request at %d for %d rows", start, n)
				}
				return instrumentPage(0, 0, "")
			})
			WithLogger(slog.New(slog.NewJSONHandler(&log, nil)))(c.native)

			rows, err := c.GetInstruments()
			switch mode {
			case "confirmed":
				if err != nil || len(rows) != end || rows[0].Code != "NVDA" || rows[end-1].Code != "SCH01805" {
					t.Fatalf("want %d readable rows, got %d: %v", end, len(rows), err)
				}
				for _, field := range []string{`"level":"WARN"`, `"server":"offline-test"`, `"reported_total":110910`, `"read_total":110893`} {
					if !strings.Contains(log.String(), field) {
						t.Errorf("warning is missing %s: %s", field, &log)
					}
				}
			case "short-continues":
				if err != nil || len(rows) != total || rows[total-1].Code != "SCH01806" || log.Len() != 0 {
					t.Fatalf("want %d rows and no warning, got %d: %v, log=%s", total, len(rows), err, &log)
				}
			default:
				if rows != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("want unexpected EOF and no rows, got %d rows: %v", len(rows), err)
				}
			}
			if checks != tc.checks {
				t.Errorf("want %d tail checks, got %d", tc.checks, checks)
			}
		})
	}
}

func TestHongKongMarketRouting(t *testing.T) {
	var quotes atomic.Int32
	native := respondingTestClient(t, true, func(header, body []byte) []byte {
		switch typ := binary.LittleEndian.Uint16(header[10:12]); typ {
		case protocol.TypeExQuote:
			quotes.Add(1)
			data := make([]byte, 150)
			copy(data, body)
			switch string(body[1:6]) {
			case "99998":
				if body[0] == 49 {
					binary.LittleEndian.PutUint32(data[14:], 0x42200000) // 前收 40
				}
			case "99997":
				binary.LittleEndian.PutUint32(data[14:], 0x42200000)
			case "99996":
				data[0] = 27 // 响应身份错误
			}
			return data
		case protocol.TypeExBars:
			code := string(body[1:6])
			if body[0] != 49 || (code != "99998" && code != "00823") {
				t.Errorf("wrong K-line identity: %x", body[:10])
			}
			if binary.LittleEndian.Uint32(body[14:18]) > 0 {
				return make([]byte, 20)
			}
			return testExtendedBar(20261118)
		default:
			t.Errorf("unexpected message type %x", typ)
			return nil
		}
	})
	unified := newMarketClient()
	t.Cleanup(func() { unified.Close() })
	unified.dial[1] = func(context.Context) (protocolClient, error) {
		return native.protocolClient, nil
	}
	// Compiled codes route directly, including on a cold connection.
	var workers sync.WaitGroup
	inputs := []string{"hk.00823", "HK00823", "00823.Hk", "00823", "香港交易所00823", "00823.香港交易所"}
	for n, input := range inputs {
		workers.Go(func() {
			var client Client = native.protocolClient
			if n%2 == 0 {
				client = unified
			}
			view := WithContext(context.Background(), client)
			got, err := view.GetQuote(input)
			if err != nil || len(got) != 1 || got[0].Instrument.Exchange != protocol.ExchangeHKFund ||
				got[0].Extended.Market != 49 || got[0].Instrument.String() != "hk.00823" {
				t.Errorf("%s: %+v, %v", input, got, err)
			}
		})
	}
	workers.Wait()
	if quotes.Load() != int32(len(inputs)) {
		t.Fatalf("expected one quote request per compiled code, got %d", quotes.Load())
	}
	bars, err := native.GetKlineDayAll("hk.99998")
	if err != nil || len(bars.List) != 1 || bars.Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(bars, err)
	}
	empty, err := native.GetHistoryTrade("20261114", "hk.00823", 0, 1)
	if err != nil || empty.Count != 0 || empty.Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(empty, err)
	}
	got, err := native.GetQuote("hk.02800", "02800.49")
	if err != nil || len(got) != 2 || got[0].Extended.Market != 49 || got[1].Extended.Market != 49 {
		t.Fatal(got, err)
	}
	got, err = native.GetQuote("hk.99998")
	if err != nil || len(got) != 1 || got[0].Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(got, err)
	}
	if quotes.Load() != int32(len(inputs)+6) {
		t.Fatalf("expected two probes per unresolved query and one request per known quote, got %d", quotes.Load())
	}
	for _, tc := range []struct{ input, want string }{
		{"hk.99997", "同时在通达信市场 31 和 49 返回有效报价"},
		{"hk.99999", "未取得可确认市场的报价（候选市场 31、49）"},
		{"hk.99996", "扩展行情响应品种与请求不一致"},
	} {
		got, err := native.GetQuote(tc.input)
		if err == nil || !strings.Contains(err.Error(), tc.want) || got != nil {
			t.Fatalf("%s: %+v %v", tc.input, got, err)
		}
	}
}

func TestHongKongImportedMarketRouting(t *testing.T) {
	dir, emptyDir := t.TempDir(), t.TempDir()
	t.Cleanup(func() {
		if err := protocol.LoadInstrumentMarkets(emptyDir); err != nil {
			t.Error(err)
		}
	})
	if err := protocol.LoadInstrumentMarkets(dir); err != nil {
		t.Fatal(err)
	}
	for _, items := range [][]protocol.ExInstrument{
		{{Market: 31, Code: "98991"}, {Market: 31, Code: "98992"}},
		{{Market: 49, Code: "98991"}, {Market: 31, Code: "98993"}, {Market: 49, Code: "98993"}, {Market: 74, Code: "98994"}},
	} {
		if err := protocol.ImportInstruments(items); err != nil {
			t.Fatal(err)
		}
	}
	checkRoutes := func() {
		t.Helper()
		for code, exchange := range map[string]protocol.Exchange{"98991": 49, "98992": 31, "98993": 49} {
			i, err := protocol.ResolveInstrument("hk."+code, nil)
			if err != nil || i.Exchange != exchange || i.Code != code {
				t.Fatalf("%s: %+v, %v", code, i, err)
			}
		}
		if i, err := protocol.ResolveInstrument("hk.98994", nil); err == nil {
			t.Fatalf("imported an unrelated market: %+v", i)
		}
	}
	checkRoutes()

	if err := protocol.LoadInstrumentMarkets(emptyDir); err != nil {
		t.Fatal(err)
	}
	if i, err := protocol.ResolveInstrument("hk.98991", nil); err == nil {
		t.Fatalf("expected the empty directory to clear learned routes: %+v", i)
	}
	if err := protocol.LoadInstrumentMarkets(dir); err != nil {
		t.Fatal(err)
	}
	checkRoutes()

	c := respondingTestClient(t, true, func(header, body []byte) []byte {
		if typ := binary.LittleEndian.Uint16(header[10:12]); typ != protocol.TypeExQuote || body[0] != 49 || string(body[1:6]) != "98991" {
			t.Errorf("expected a direct market 49 quote, got type %x body %x", typ, body)
		}
		data := make([]byte, 150)
		copy(data, body)
		return data
	})
	if quotes, err := c.GetQuote("hk.98991"); err != nil || len(quotes) != 1 || quotes[0].Instrument.Exchange != 49 {
		t.Fatal(quotes, err)
	}

	file := filepath.Join(dir, "hk-instruments.json")
	if err := os.Rename(file, file+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := protocol.ImportInstruments([]protocol.ExInstrument{{Market: 31, Code: "98991"}}); err == nil {
		t.Fatal("expected the file replacement to fail")
	}
	checkRoutes()
}
