package tdx

import (
	"encoding/binary"
	"math"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

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
			if c.queryClient() != c.queryClient() {
				t.Fatal("query metadata is not reused")
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
