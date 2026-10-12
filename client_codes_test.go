package tdx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

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

func TestUnifiedMarketDiscovery(t *testing.T) {
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { t.Error("static markets sent request"); return nil })
	r, err := std.GetMarkets()
	if err != nil || len(r) != 3 || r[2].ShortName != "bj" || r[2].Extended != nil {
		t.Fatal(r, err)
	}
	c := marketTestClient(t, nil, func(h, _ []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) != protocol.TypeExMarkets {
			t.Error("unexpected request")
		}
		return testMarketTable()
	})
	r, err = c.GetMarkets()
	if err != nil || len(r) != 4 || r[3].Market != 200 || r[3].Protocol != protocol.ProtocolExtended || r[3].Extended.Category != 13 {
		t.Fatal(r, err)
	}
	if c.links[0] != nil {
		t.Fatal("discovery opened standard connection")
	}
	if _, err = c.GetCodeAll(200); err == nil {
		t.Fatal("discovered market silently enabled routing")
	}
	c.Close()
	if _, err = c.GetMarkets(); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
	std.Close()
	if _, err = std.GetMarkets(); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
	failed := newMarketClient()
	t.Cleanup(func() { failed.Close() })
	failed.dial[1] = func() (protocolClient, error) { return nil, io.ErrUnexpectedEOF }
	if r, err = failed.GetMarkets(); !errors.Is(err, io.ErrUnexpectedEOF) || r != nil {
		t.Fatal("returned incomplete discovery", r, err)
	}
}
