package tdx

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func marketTestClient(t *testing.T, std, ex func([]byte, []byte) []byte) *MarketClient {
	t.Helper()
	c := newMarketClient()
	for j, serve := range []func([]byte, []byte) []byte{std, ex} {
		index, respond := j, serve
		c.dial[index] = func() (*Conn, error) {
			if respond == nil {
				t.Error("unexpected connection")
				return nil, errors.New("unexpected connection")
			}
			return respondingTestClient(t, index == 1, respond), nil
		}
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func testStandardBar(index bool) []byte {
	// 一根日线，价格 10 元，量 100，额 1000；指数另带涨跌家数。
	b := []byte{1, 0}
	b = append(b, protocol.Bytes(uint32(20261009))...)
	b = append(b, 0x90, 0x9c, 0x01, 0, 0, 0) // 10000 厘
	b = append(b, protocol.Bytes(math.Float32bits(100))...)
	b = append(b, protocol.Bytes(math.Float32bits(1000))...)
	if index {
		b = append(b, 3, 0, 2, 0)
	}
	return b
}

func testExtendedBar(day uint32) []byte {
	b := make([]byte, 52)
	binary.LittleEndian.PutUint16(b[18:20], 1)
	binary.LittleEndian.PutUint32(b[20:24], day)
	for p, v := range map[int]float32{24: 10.25, 28: 11.25, 32: 9.25, 36: 10.5, 48: 10.75} {
		binary.LittleEndian.PutUint32(b[p:p+4], math.Float32bits(v))
	}
	binary.LittleEndian.PutUint32(b[40:44], 123)
	binary.LittleEndian.PutUint32(b[44:48], 456)
	return b
}

func TestMarketClientKlineRouting(t *testing.T) {
	c := marketTestClient(t, func(h, b []byte) []byte {
		if h[0] != 0x0c || binary.LittleEndian.Uint16(h[10:12]) != protocol.TypeKline || b[8] != 9 {
			t.Error("wrong standard request", h, b)
		}
		return testStandardBar(string(b[2:8]) == "000001" && b[0] == 1)
	}, func(h, b []byte) []byte {
		if h[0] != 1 || binary.LittleEndian.Uint16(h[10:12]) != protocol.TypeExBars {
			t.Error("wrong extended request", h)
		}
		want := uint16(4)
		if b[0] == 27 || b[0] == 74 {
			want = 9
		}
		if binary.LittleEndian.Uint16(b[10:12]) != want {
			t.Error("wrong day category", b)
		}
		return testExtendedBar(20261009)
	})
	for _, tc := range []struct {
		code string
		unit protocol.VolumeUnit
		vol  int64
	}{
		{"sz000001", protocol.VolumeHundredShares, 100},
		{"sh000001", protocol.VolumeShares, 10000},
		{"hk00700", protocol.VolumeHundredShares, 456},
		{"cffIF2609", VolumeUnknown, 456}, {"hk.HSI", VolumeUnknown, 456}, {"usAAPL", VolumeUnknown, 456},
	} {
		r, err := c.GetKlineDay(tc.code, 0, 1)
		if err != nil || r.Count != 1 || r.VolumeUnit != tc.unit || r.List[0].Volume != tc.vol {
			t.Fatal(tc.code, r, err)
		}
		k := r.List[0]
		if k.Standard != nil {
			if k.Close != 10 || k.Amount == nil || *k.Amount != 1000 {
				t.Fatal("standard prices/amount changed", k)
			}
		} else if k.Close != 10.5 || k.Extended.Position != 123 || k.Extended.Price != 10.75 || k.Amount != nil {
			t.Fatal("lost futures fields or mislabeled position as amount", k)
		}
	}
}

func testQuoteResponse(b []byte) []byte {
	n := int(binary.LittleEndian.Uint16(b[8:10]))
	out := append([]byte{1, 0x36}, protocol.Bytes(uint16(n))...)
	for j := 0; j < n; j++ {
		prefix := b[10+7*j : 17+7*j]
		out = append(out, prefix...)
		out = append(out, 0, 0)                // 活跃度
		out = append(out, 0xa8, 0x0f)          // 10 元
		out = append(out, make([]byte, 12)...) // 4 价差 + 时间/保留/量/现量 + 金额
		out = append(out, make([]byte, 24)...) // 内外盘/保留 + 五档 20 个变长整数
		out = append(out, make([]byte, 10)...) // 末尾保留字段
	}
	return out
}

func TestMarketClientMixedQuotesAndMetadata(t *testing.T) {
	var codeRequests atomic.Int32
	c := marketTestClient(t, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeCode {
			codeRequests.Add(1)
			data := make([]byte, 31)
			binary.LittleEndian.PutUint16(data, 1)
			copy(data[2:8], "110075")
			data[22] = 4 // 转债价格精度校准：10 元原始值除以 100
			return data
		}
		return testQuoteResponse(b)
	}, func(_ []byte, b []byte) []byte {
		data := make([]byte, 150)
		copy(data[:10], b)
		binary.LittleEndian.PutUint32(data[30:34], math.Float32bits(500))
		return data
	})
	inputs := []string{"hk00700", "sz000001", "usAAPL", "sh110075", "sz000001"}
	for repeat := 0; repeat < 2; repeat++ {
		r, err := c.GetQuote(inputs...)
		if err != nil || len(r) != len(inputs) {
			t.Fatal(r, err)
		}
		for j, input := range inputs {
			i, _ := protocol.ParseInstrument(input)
			if r[j].Instrument != i {
				t.Fatal("mixed quote order lost", r)
			}
		}
		if r[0].Price != 500 || r[1].Price != 10 || r[3].Price != 0.1 || r[4].Price != 10 {
			t.Fatal("price calibration or duplicate quotes changed", r)
		}
	}
	if codeRequests.Load() != 1 {
		t.Fatal("code precision metadata was fetched more than once", codeRequests.Load())
	}
}

func TestMarketClientTicks(t *testing.T) {
	c := marketTestClient(t, func(h, _ []byte) []byte {
		switch binary.LittleEndian.Uint16(h[10:12]) {
		case protocol.TypeKline:
			return testStandardBar(false)
		case protocol.TypeHistoryMinute:
			return []byte{1, 0, 0, 0, 0, 0, 0xa8, 0x0f, 0, 2}
		case protocol.TypeTrade:
			return []byte{1, 0, 0x3a, 2, 0xa8, 0x0f, 1, 3, 1, 0}
		default:
			return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 1, 0}
		}
	}, func(h, _ []byte) []byte {
		typ := binary.LittleEndian.Uint16(h[10:12])
		if typ == protocol.TypeExBars {
			return testExtendedBar(20261009)
		}
		if typ == protocol.TypeExMinute || typ == protocol.TypeExHistMinute {
			header := 12
			if typ == protocol.TypeExHistMinute {
				header = 20
			}
			b := make([]byte, header+18)
			binary.LittleEndian.PutUint16(b[header-2:header], 1)
			binary.LittleEndian.PutUint16(b[header:header+2], 570)
			binary.LittleEndian.PutUint32(b[header+2:header+6], math.Float32bits(500))
			binary.LittleEndian.PutUint32(b[header+6:header+10], math.Float32bits(499))
			binary.LittleEndian.PutUint32(b[header+14:], 123)
			return b
		}
		b := make([]byte, 32)
		binary.LittleEndian.PutUint16(b[14:16], 1)
		binary.LittleEndian.PutUint16(b[16:18], 570)
		binary.LittleEndian.PutUint32(b[18:22], 500000)
		binary.LittleEndian.PutUint32(b[22:26], 100)
		binary.LittleEndian.PutUint16(b[30:32], 256) // 港股卖出
		return b
	})
	for _, code := range []string{"sz000001", "hk00700"} {
		for _, historical := range []bool{false, true} {
			minute, err := c.GetMinute(code)
			if historical {
				minute, err = c.GetHistoryMinute("20261009", code)
			}
			if err != nil || minute.Count != 1 {
				t.Fatal(minute, err)
			}
			trade, err := c.GetTrade(code, 0, 1)
			if historical {
				trade, err = c.GetHistoryTrade("20261009", code, 0, 1)
			}
			if err != nil || trade.Count != 1 || trade.List[0].Direction != -1 {
				t.Fatal(trade, err)
			}
			if code == "hk00700" {
				if minute.List[0].Price != 500 || minute.List[0].Extended.OpenInterest != 123 || trade.List[0].Price != nil || trade.List[0].Extended.Price != 500000 {
					t.Fatal("extended fields/scales changed", minute, trade)
				}
			} else if trade.List[0].Price == nil || *trade.List[0].Price != 10 {
				t.Fatal("standard price changed", trade)
			}
		}
	}
}

func TestMarketClientCodesFilterGlobalPages(t *testing.T) {
	var pages atomic.Int32
	c := marketTestClient(t, nil, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeExCount {
			data := make([]byte, 23)
			binary.LittleEndian.PutUint32(data[19:23], 801)
			return data
		}
		pages.Add(1)
		start := binary.LittleEndian.Uint32(b[:4])
		n := int(binary.LittleEndian.Uint16(b[4:6]))
		if (start != 0 && start != 800) || (start == 800 && n != 1) {
			t.Error("wrong global page", start, n)
		}
		data := make([]byte, 6+n*64)
		binary.LittleEndian.PutUint16(data[4:6], uint16(n))
		for j := 0; j < n; j++ {
			p := 6 + j*64
			data[p+1] = 74
			copy(data[p+5:p+14], "AAPL")
		}
		data[7] = 31 // 每页一个港股，其余为美股
		copy(data[11:20], "00700\x00\x00\x00\x00")
		return data
	})
	r, err := c.GetCodeAll(protocol.ExchangeHK)
	if err != nil || r.Count != 2 || pages.Load() != 2 {
		t.Fatal(r, err, pages.Load())
	}
	n, err := c.GetCount(protocol.ExchangeHK)
	if err != nil || n != 2 {
		t.Fatal("returned global ExCount as market count", n, err)
	}
}

func TestMarketClientLazyLifecycleAndFailures(t *testing.T) {
	c := newMarketClient()
	var dials atomic.Int32
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { return []byte{5, 0} })
	c.dial[0] = func() (*Conn, error) { dials.Add(1); return std, nil }
	c.dial[1] = func() (*Conn, error) {
		t.Error("unused extended protocol was connected")
		return nil, io.ErrClosedPipe
	}
	c.SetTimeout(time.Second)
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if n, err := c.GetCount(protocol.ExchangeSZ); err != nil || n != 5 {
				t.Error(n, err)
			}
		}()
	}
	wg.Wait()
	if dials.Load() != 1 || std.waitTimeout != time.Second {
		t.Fatal("lazy connection was duplicated or lost timeout")
	}
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Close() }()
	}
	wg.Wait()
	if _, err := c.GetCount(protocol.ExchangeSZ); !errors.Is(err, net.ErrClosed) || dials.Load() != 1 {
		t.Fatal("closed facade reopened", err)
	}
	f := marketTestClient(t, func(_ []byte, b []byte) []byte {
		return testQuoteResponse(b)
	}, func(_, _ []byte) []byte { return nil })
	if r, err := f.GetQuote("sz000001", "hk00700"); err == nil || r != nil {
		t.Fatal("mixed failure returned partial quotes", r, err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	for _, link := range f.links {
		if link == nil {
			t.Fatal("mixed request did not reach both connections")
		}
		select {
		case <-link.Done():
		default:
			t.Error("facade left a connection running")
		}
	}
}

func TestMarketClientValidationBeforeDial(t *testing.T) {
	c := marketTestClient(t, nil, nil)
	oversized := make([]string, 9361)
	for j := range oversized {
		oversized[j] = "sz000001"
	}
	checks := []func() error{
		func() error { _, e := c.GetKlineDay("hk.1234567890", 0, 1); return e },
		func() error { _, e := c.GetKlineDay("sz000001", 0, 801); return e },
		func() error { _, e := c.GetKline(12, "sz000001", 0, 1); return e },
		func() error { _, e := c.GetHistoryMinute("20260230", "hk00700"); return e },
		func() error { _, e := c.GetHistoryTrade("", "hk00700", 0, 1); return e },
		func() error { _, e := c.GetTrade("sz000001", 0, 0); return e },
		func() error { _, e := c.GetQuote("sz000001", "hk.invalid$"); return e },
		func() error { _, e := c.GetCodeAll(255); return e },
		func() error { _, e := c.GetQuote(oversized...); return e },
	}
	for j, check := range checks {
		if err := check(); err == nil {
			t.Errorf("validation %d passed", j)
		}
	}
}

func TestMarketClientMinuteCategories(t *testing.T) {
	c := marketTestClient(t, func(_ []byte, b []byte) []byte {
		if b[8] != 7 {
			t.Error("standard minute category", b[8])
		}
		return []byte{0, 0}
	}, func(_ []byte, b []byte) []byte {
		if binary.LittleEndian.Uint16(b[10:12]) != 8 {
			t.Error("extended minute category", b)
		}
		return make([]byte, 20)
	})
	for _, code := range []string{"sz000001", "sh000001", "hk00700", "cffIF2609", "usAAPL"} {
		r, err := c.GetKlineMinute(code, 0, 1)
		if err != nil || r.Count != 0 {
			t.Fatal(code, r, err)
		}
	}
}

func TestMarketClientDialFailureDoesNotPoisonOtherProtocol(t *testing.T) {
	c := newMarketClient()
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { return []byte{2, 0} })
	c.dial[0] = func() (*Conn, error) { return std, nil }
	c.dial[1] = func() (*Conn, error) { return nil, io.ErrClosedPipe }
	defer c.Close()
	if _, err := c.GetKlineDay("hk00700", 0, 1); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	if n, err := c.GetCount(protocol.ExchangeSZ); err != nil || n != 2 {
		t.Fatal(n, err)
	}
}

func TestMarketClientCapturedHKDay(t *testing.T) {
	const raw = "1f30303730300000000004000100000000000100912835010180cf4367e6d4436766cf436766d443064e0050c11d0300d07c0f4a"
	data, err := hex.DecodeString(raw)
	if err != nil {
		t.Fatal(err)
	}
	c := marketTestClient(t, nil, func(_ []byte, b []byte) []byte {
		if b[0] != 31 || binary.LittleEndian.Uint16(b[10:12]) != 4 {
			t.Error("captured request mismatch")
		}
		return data
	})
	r, err := c.GetKlineDay("hk00700", 0, 1)
	if err != nil || r.Count != 1 || r.List[0].Volume != 204225 || r.VolumeUnit != protocol.VolumeHundredShares {
		t.Fatal("captured Tencent cross-validation sample changed", r, err)
	}
	t.Logf("2026-10-09 HK00700: volume=%d hundred_shares; independent saved sample=20422500 shares", r.List[0].Volume)
}

func TestMarketClientKlinePagination(t *testing.T) {
	var calls atomic.Int32
	c := marketTestClient(t, nil, func(_, b []byte) []byte {
		calls.Add(1)
		start := int(binary.LittleEndian.Uint32(b[14:18]))
		n := 800
		if start != 0 {
			n = 1
		}
		data := make([]byte, 20+n*32)
		binary.LittleEndian.PutUint16(data[18:20], uint16(n))
		for j := 0; j < n; j++ {
			bar := testExtendedBar(20261009)
			binary.LittleEndian.PutUint32(bar[44:48], uint32(1000-start-n+j))
			copy(data[20+j*32:], bar[20:])
		}
		return data
	})
	r, err := c.GetKlineUntil(9, "hk00700", func(k MarketKline) bool { return k.Volume == 199 })
	if err != nil || r.Count != 801 || r.List[0].Volume != 199 || r.List[800].Volume != 999 || calls.Load() != 2 {
		t.Fatal("pagination order/inclusive boundary changed", r, err)
	}
}
