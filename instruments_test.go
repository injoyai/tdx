package tdx

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
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

func TestHongKongSnapshotRouting(t *testing.T) {
	items := []protocol.ExInstrument{
		{Market: 31, Code: "00700"},
		{Market: 49, Code: "00823"},
		{Market: 31, Code: "01234"},
		{Market: 49, Code: "01234"},
		// This symbol is in an ETF code range but belongs to 31 in this fixture.
		{Market: 31, Code: "02800"},
	}
	var scans, quotes atomic.Int32
	native := respondingTestClient(t, true, func(header, body []byte) []byte {
		switch typ := binary.LittleEndian.Uint16(header[10:12]); typ {
		case protocol.TypeExCount:
			scans.Add(1)
			data := make([]byte, 23)
			binary.LittleEndian.PutUint32(data[19:23], uint32(len(items)))
			return data
		case protocol.TypeExInstrument:
			data := make([]byte, 6+64*len(items))
			binary.LittleEndian.PutUint16(data[4:6], uint16(len(items)))
			for j, item := range items {
				offset := 6 + 64*j
				data[offset+1] = item.Market
				copy(data[offset+5:offset+14], item.Code)
			}
			return data
		case protocol.TypeExQuote:
			quotes.Add(1)
			data := make([]byte, 150)
			copy(data, body)
			return data
		case protocol.TypeExBars:
			if body[0] != 49 || string(body[1:6]) != "00823" {
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
	// Request views share one lazy snapshot even when the connection is cold.
	var workers sync.WaitGroup
	for n, input := range []string{"hk.00823", "HK00823", "00823.Hk", "00823", "hk/00823"} {
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
	if scans.Load() != 1 || quotes.Load() != 5 {
		t.Fatalf("snapshot scans=%d quotes=%d", scans.Load(), quotes.Load())
	}
	bars, err := native.GetKlineDay("hk.00823", 0, 1)
	if err != nil || len(bars.List) != 1 || bars.Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(bars, err)
	}
	empty, err := native.GetHistoryTrade("20261114", "hk.00823", 0, 1)
	if err != nil || empty.Count != 0 || empty.Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(empty, err)
	}
	got, err := native.GetQuote("hk.02800", "02800.49")
	if err != nil || len(got) != 2 || got[0].Extended.Market != 31 || got[1].Extended.Market != 49 {
		t.Fatal(got, err)
	}
	for _, tc := range []struct{ input, want string }{
		{"hk.01234", "同时出现在通达信市场 31 和 49"},
		{"hk.00042", "未找到（候选市场 31、49）"},
	} {
		got, err := native.GetQuote(tc.input)
		if err == nil || !strings.Contains(err.Error(), tc.want) || got != nil {
			t.Fatalf("%s: %+v %v", tc.input, got, err)
		}
	}
	if scans.Load() != 1 || quotes.Load() != 7 {
		t.Fatalf("ambiguous or unknown code sent a request: scans=%d quotes=%d", scans.Load(), quotes.Load())
	}
}
