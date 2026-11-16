package tdx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"strings"
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
