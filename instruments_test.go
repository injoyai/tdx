package tdx

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestInstrumentsGlobalPaginationAndCompleteness(t *testing.T) {
	for _, mode := range []string{"complete", "empty", "missing", "oversized"} {
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
				if (start == 0 && n != 800) || (start == 800 && n != 1) || start > 800 {
					t.Error("incorrect global pagination", start, n)
				}
				if mode == "missing" && start == 800 {
					n = 0
				}
				if mode == "oversized" {
					n++
				}
				data := make([]byte, 6+n*64)
				binary.LittleEndian.PutUint16(data[4:6], uint16(n))
				for j := 0; j < n; j++ {
					p := 6 + j*64
					data[p+1] = byte(protocol.ExchangeUS)
					copy(data[p+5:p+14], "AAPL")
				}
				return data
			})
			rows, err := c.GetInstruments()
			if mode == "missing" || mode == "oversized" {
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
