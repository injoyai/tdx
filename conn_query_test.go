package tdx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"reflect"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestCodeListsAndReports(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range []string{protocol.FileTdxBjMore, protocol.FileTdxZs, protocol.FileTdxBk, protocol.FileTdxStat, protocol.FileTdxStat2, protocol.FileXgsg, protocol.FileSpBlock} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name == protocol.FileTdxBjMore {
			if _, err = w.Write([]byte("44|920001|2|BJ|1|\n")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	exchanges := make(chan uint16, 10)
	var archive atomic.Value
	archive.Store(append([]byte(nil), buf.Bytes()...))
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		switch binary.LittleEndian.Uint16(h[10:]) {
		case protocol.TypeBlockInfo:
			raw := archive.Load().([]byte)
			return append(protocol.Bytes(uint32(len(raw))), raw...)
		case protocol.TypeCode:
			ex := binary.LittleEndian.Uint16(b)
			exchanges <- ex
			codes := []string{"600000", "510300", "000001", "880001"}
			if ex == 0 {
				codes = []string{"000001", "159001", "399001"}
			}
			data := make([]byte, 2+29*len(codes))
			binary.LittleEndian.PutUint16(data, uint16(len(codes)))
			for i, code := range codes {
				copy(data[2+29*i:], code)
			}
			return data
		default:
			t.Errorf("unexpected request %x", h)
			return nil
		}
	})
	for _, tc := range []struct {
		name string
		get  func() ([]string, error)
		want []string
	}{
		{"stocks", c.GetStockCodeAll, []string{"sh600000", "sz000001", "bj920001"}},
		{"etfs", c.GetETFCodeAll, []string{"sh510300", "sz159001"}},
		{"indexes", c.GetIndexCodeAll, []string{"bj899050", "sh000001", "sh880001", "sz399001"}},
	} {
		got, err := tc.get()
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc.name, got, err)
		}
		if got := []uint16{<-exchanges, <-exchanges}; !reflect.DeepEqual(got, []uint16{1, 0}) {
			t.Fatal("changed standard exchange range/order", got)
		}
		t.Logf("fixture %s: %v", tc.name, got)
	}
	for _, get := range []func() error{
		func() error { _, err := c.GetTdxZs(); return err }, func() error { _, err := c.GetTdxBk(); return err },
		func() error { _, err := c.GetTdxStat(); return err }, func() error { _, err := c.GetTdxStat2(); return err },
		func() error { _, err := c.GetXgsg(); return err }, func() error { _, err := c.GetSpBlock(); return err },
	} {
		if err := get(); err != nil {
			t.Fatal(err)
		}
	}
	buf.Reset()
	z = zip.NewWriter(&buf)
	if _, err := z.Create("unrelated"); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	archive.Store(append([]byte(nil), buf.Bytes()...))
	for _, tc := range []struct {
		name string
		get  func() error
	}{
		{protocol.FileTdxZs, func() error { _, e := c.GetTdxZs(); return e }},
		{protocol.FileTdxBk, func() error { _, e := c.GetTdxBk(); return e }},
		{protocol.FileTdxStat, func() error { _, e := c.GetTdxStat(); return e }},
		{protocol.FileTdxStat2, func() error { _, e := c.GetTdxStat2(); return e }},
		{protocol.FileXgsg, func() error { _, e := c.GetXgsg(); return e }},
		{protocol.FileSpBlock, func() error { _, e := c.GetSpBlock(); return e }},
	} {
		want := fmt.Sprintf("%s 中缺少 %s", protocol.ReportZHB, tc.name)
		if err := tc.get(); err == nil || err.Error() != want {
			t.Fatal(tc.name, err, want)
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

func TestFileChunkLengthValidation(t *testing.T) {
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
			return protocol.Bytes(uint32(30001))
		}
		return make([]byte, 4+30001)
	})
	for _, get := range []func(string) ([]byte, error){c.GetReportFile, c.GetBlockFileRaw} {
		if data, err := get("sample"); err == nil || data != nil {
			t.Fatal("oversized chunk accepted", len(data), err)
		}
	}
}

func TestFileChunkTermination(t *testing.T) {
	for _, size := range []int{0, 17, 30000, 30017} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			want := bytes.Repeat([]byte{7}, size)
			c := respondingTestClient(t, false, func(h, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
					return protocol.Bytes(uint32(size))
				}
				start, count := int(binary.LittleEndian.Uint32(b)), int(binary.LittleEndian.Uint32(b[4:]))
				end := min(start+count, size)
				return append(protocol.Bytes(uint32(end-start)), want[start:end]...)
			})
			got, err := c.GetReportFile("sample")
			if err != nil || !bytes.Equal(got, want) {
				t.Fatal("report differs", len(got), err)
			}
			got, err = c.GetBlockFileRaw("sample")
			if size == 0 {
				if err == nil {
					t.Fatal("empty metadata accepted")
				}
			} else if err != nil || !bytes.Equal(got, want) {
				t.Fatal("block differs", len(got), err)
			}
		})
	}
	// 已知大小的文件不能将提前遇到空块当作成功 EOF。
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
			return protocol.Bytes(uint32(17))
		}
		return make([]byte, 4)
	})
	if _, err := c.GetBlockFileRaw("truncated"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("missing data not reported", err)
	}
}

func TestGetBlockDataPropagatesParseErrors(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		size, count, components int
		valid                   bool
	}{
		{"complete", 386 + 2813, 1, 1, true},
		{"empty", 386, 0, 0, true},
		{"missing component region", 399, 1, 1, false},
		{"oversized component count", 386 + 2813, 1, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.size)
			binary.LittleEndian.PutUint16(data[384:], uint16(tc.count))
			if tc.count > 0 {
				copy(data[386:], "Sample")
				binary.LittleEndian.PutUint16(data[395:], uint16(tc.components))
				copy(data[399:], "1600000")
			}
			c := respondingTestClient(t, false, func(h, request []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
					return protocol.Bytes(uint32(len(data)))
				}
				start, size := int(binary.LittleEndian.Uint32(request)), int(binary.LittleEndian.Uint32(request[4:]))
				chunk := data[start:min(start+size, len(data))]
				return append(protocol.Bytes(uint32(len(chunk))), chunk...)
			})
			got, err := c.GetBlockData("sample.dat")
			if !tc.valid {
				if err == nil || got != nil {
					t.Fatal("malformed block data reported as success", got, err)
				}
				return
			}
			if err != nil || len(got) != tc.count {
				t.Fatal("valid block data changed", got, err)
			}
			if tc.count > 0 && (got[0].Name != "Sample" || len(got[0].Codes) != 1 || got[0].Codes[0] != "1600000") {
				t.Fatal("block fields changed", got)
			}
		})
	}
}
