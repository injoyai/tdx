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

	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

func marketTestClient(t *testing.T, std, ex func([]byte, []byte) []byte) *MarketClient {
	t.Helper()
	c := newMarketClient()
	for j, serve := range []func([]byte, []byte) []byte{std, ex} {
		index, respond := j, serve
		c.dial[index] = func() (protocolClient, error) {
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
		vol  int64
	}{
		{"sz000001", 10000},
		{"sh000001", 10000},
		{"hk00700", 45600},
		{"cffIF2609", 456}, {"usAAPL", 456},
	} {
		r, err := c.GetKlineDay(tc.code, 0, 1)
		if err != nil || r.Count != 1 || r.List[0].Volume != tc.vol {
			t.Fatal(tc.code, r, err)
		}
		k := r.List[0]
		if len(k.Time) != 10 {
			t.Fatal("daily source date must not include a fabricated close time", k.Time)
		}
		if _, err := k.Timestamp(protocol.Location()); err == nil {
			t.Fatal("daily label converted to actual instant")
		}
		if k.Standard != nil {
			if r.TimeZone != "UTC+08:00" || k.TradingDate != k.Time {
				t.Fatal("lost standard source timezone/trading date", r.TimeZone, k)
			}
			if k.Close != 10 || k.Amount == nil || *k.Amount != 1000 {
				t.Fatal("standard prices/amount changed", k)
			}
		} else if k.Close != 10.5 || k.Extended.Position != 123 || k.Extended.Price != 10.75 || k.Amount != nil {
			t.Fatal("lost futures fields or mislabeled position as amount", k)
		}
		if k.Extended != nil && (r.TimeZone != "" || k.TradingDate != "") {
			t.Fatal("guessed extended timezone or trading date", r.TimeZone, k)
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
	}, func(h, request []byte) []byte {
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
			binary.LittleEndian.PutUint32(b[header+10:header+14], 123)
			binary.LittleEndian.PutUint32(b[header+14:], 123)
			return b
		}
		b := make([]byte, 32)
		binary.LittleEndian.PutUint16(b[14:16], 1)
		binary.LittleEndian.PutUint16(b[16:18], 570)
		binary.LittleEndian.PutUint32(b[18:22], 500000)
		binary.LittleEndian.PutUint32(b[22:26], 100)
		market := request[0]
		if typ == protocol.TypeExHistTrade {
			market = request[4]
		}
		nature := uint16(256)
		if market != 31 {
			nature = 10000
		}
		binary.LittleEndian.PutUint16(b[30:32], nature)
		return b
	})
	for _, code := range []string{"sz000001", "hk00700", "cffIF2609", "usAAPL"} {
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
			wantMinute, wantTrade := int64(123), int64(100)
			if code == "sz000001" {
				wantMinute, wantTrade = 200, 100
			}
			if minute.List[0].Volume != wantMinute || trade.List[0].Volume != wantTrade {
				t.Fatal("tick minimum units changed", code, minute, trade)
			}
			if code != "sz000001" {
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
	c.dial[0] = func() (protocolClient, error) { dials.Add(1); return std, nil }
	c.dial[1] = func() (protocolClient, error) {
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
	c.dial[0] = func() (protocolClient, error) { return std, nil }
	c.dial[1] = func() (protocolClient, error) { return nil, io.ErrClosedPipe }
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
	if err != nil || r.Count != 1 || r.List[0].Volume != 20422500 {
		t.Fatal("captured Tencent cross-validation sample changed", r, err)
	}
	t.Logf("2026-10-09 HK00700: volume=%d shares; independent saved sample=20422500 shares", r.List[0].Volume)
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
	r, err := c.GetKlineUntil(9, "hk00700", func(k Kline) bool { return k.Volume == 19900 })
	if err != nil || r.Count != 801 || r.List[0].Volume != 19900 || r.List[800].Volume != 99900 || calls.Load() != 2 {
		t.Fatal("pagination order/inclusive boundary changed", r, err)
	}
}

func TestMarketClientSlowDialKeepsOtherProtocolUsable(t *testing.T) {
	c := newMarketClient()
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { return []byte{2, 0} })
	c.dial[0] = func() (protocolClient, error) { return std, nil }
	if _, err := c.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	dialDone := make(chan error, 1)
	c.dial[1] = func() (protocolClient, error) {
		close(started)
		<-release
		return nil, io.ErrClosedPipe
	}
	go func() { _, err := c.GetKlineDay("hk00700", 0, 1); dialDone <- err }()
	<-started
	t.Cleanup(func() { close(release); <-dialDone; c.Close() })
	queryDone := make(chan error, 1)
	go func() { _, err := c.GetCount(protocol.ExchangeSZ); queryDone <- err }()
	select {
	case err := <-queryDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow extended dial blocked the existing standard connection")
	}
	setDone := make(chan struct{})
	go func() { c.SetTimeout(3 * time.Second); close(setDone) }()
	select {
	case <-setDone:
	case <-time.After(time.Second):
		t.Fatal("slow extended dial blocked SetTimeout")
	}
}

func TestMarketClientCloseDuringDialDiscardsLateConnection(t *testing.T) {
	c := newMarketClient()
	std := respondingTestClient(t, false, func(_, _ []byte) []byte { return []byte{2, 0} })
	ex := respondingTestClient(t, true, func(_, _ []byte) []byte { return make([]byte, 20) })
	c.dial[0] = func() (protocolClient, error) { return std, nil }
	if _, err := c.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	dialDone := make(chan error, 1)
	c.dial[1] = func() (protocolClient, error) { close(started); <-release; return ex, nil }
	go func() { _, err := c.GetKlineDay("hk00700", 0, 1); dialDone <- err }()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- c.Close() }()
	var released sync.Once
	releaseDial := func() { released.Do(func() { close(release) }) }
	t.Cleanup(func() { releaseDial(); c.Close() })
	select {
	case <-std.Done():
	case <-time.After(time.Second):
		t.Fatal("Close did not release the existing connection during a slow dial")
	}
	select {
	case <-closeDone:
		t.Fatal("Close returned before the pending dial could be cleaned up")
	default:
	}
	releaseDial()
	if err := <-dialDone; !errors.Is(err, net.ErrClosed) {
		t.Fatal("late connection was published", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-ex.Done():
	default:
		t.Fatal("late connection leaked")
	}
	if c.links[1] != nil {
		t.Fatal("closed facade retained the late connection")
	}
}

func TestMarketClientTimeoutChangedDuringDial(t *testing.T) {
	c := newMarketClient()
	ex := respondingTestClient(t, true, func(_, _ []byte) []byte { return make([]byte, 20) })
	started, release := make(chan struct{}), make(chan struct{})
	c.dial[1] = func() (protocolClient, error) { close(started); <-release; return ex, nil }
	dialDone := make(chan error, 1)
	go func() { _, err := c.connection(protocol.ExchangeHK); dialDone <- err }()
	<-started
	var released sync.Once
	releaseDial := func() { released.Do(func() { close(release) }) }
	t.Cleanup(func() { releaseDial(); c.Close() })
	setDone := make(chan struct{})
	go func() { c.SetTimeout(3 * time.Second); close(setDone) }()
	select {
	case <-setDone:
	case <-time.After(time.Second):
		t.Fatal("SetTimeout blocked during dial")
	}
	releaseDial()
	if err := <-dialDone; err != nil {
		t.Fatal(err)
	}
	if ex.waitTimeout != 3*time.Second {
		t.Fatal("new connection lost the latest timeout", ex.waitTimeout)
	}
}

func TestMarketClientCloseDuringTypedNilDial(t *testing.T) {
	c := newMarketClient()
	started, release := make(chan struct{}), make(chan struct{})
	c.dial[0] = func() (protocolClient, error) {
		close(started)
		<-release
		return (*StConn)(nil), nil
	}
	queryDone := make(chan error, 1)
	go func() { _, err := c.GetCount(protocol.ExchangeSZ); queryDone <- err }()
	<-started
	closeDone := make(chan error, 1)
	go func() { closeDone <- c.Close() }()
	// Wait until Close has marked the facade closed before releasing the dial.
	for {
		c.mu.Lock()
		closed := c.closed
		c.mu.Unlock()
		if closed {
			break
		}
		time.Sleep(time.Millisecond)
	}
	close(release)
	if err := <-queryDone; !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}

func interfaceFinanceResponse(b []byte) []byte {
	r := make([]byte, 145)
	r[2] = b[2]
	copy(r[3:9], b[3:9])
	binary.LittleEndian.PutUint32(r[9:], math.Float32bits(2))
	binary.LittleEndian.PutUint32(r[25:], math.Float32bits(3))
	return r
}

func TestClientInterfaceRoutesStandardCapabilities(t *testing.T) {
	api := NewClient(WithRedial(false))
	m, ok := api.(*MarketClient)
	if !ok {
		t.Fatal("factory did not return unified implementation")
	}
	t.Cleanup(func() { api.Close() })
	var std, ex atomic.Int32
	m.dial[0] = func() (protocolClient, error) {
		return respondingTestClient(t, false, func(h, b []byte) []byte {
			std.Add(1)
			switch binary.LittleEndian.Uint16(h[10:]) {
			case protocol.TypeQuote:
				return testQuoteResponse(b)
			case protocol.TypeFinance:
				if string(b[3:9]) != "000001" {
					t.Error("finance code changed", b)
				}
				return interfaceFinanceResponse(b)
			case protocol.TypeCompanyCat:
				r := make([]byte, 154)
				r[0] = 1
				copy(r[2:], "About")
				copy(r[66:], "info.txt")
				binary.LittleEndian.PutUint32(r[146:], 7)
				binary.LittleEndian.PutUint32(r[150:], 4)
				return r
			case protocol.TypeCompanyContent:
				if string(b[2:8]) != "000001" || binary.LittleEndian.Uint32(b[90:]) != 7 || binary.LittleEndian.Uint32(b[94:]) != 4 {
					t.Error("F10 request changed", b)
				}
				r := make([]byte, 16)
				binary.LittleEndian.PutUint16(r[10:], 4)
				copy(r[12:], "info")
				return r
			case protocol.TypeGbbq:
				return make([]byte, 11)
			case protocol.TypeCallAuction:
				return []byte{0, 0}
			case protocol.TypeBlockInfo:
				return []byte{4, 0, 0, 0, 'd', 'a', 't', 'a'}
			default:
				t.Error("unexpected standard frame", h)
				return nil
			}
		}), nil
	}
	m.dial[1] = func() (protocolClient, error) {
		return respondingTestClient(t, true, func(h, b []byte) []byte {
			ex.Add(1)
			if binary.LittleEndian.Uint16(h[10:]) != protocol.TypeExQuote {
				t.Error("standard operation sent to extension")
			}
			r := make([]byte, 150)
			copy(r, b)
			binary.LittleEndian.PutUint32(r[30:], math.Float32bits(10))
			return r
		}), nil
	}
	api.SetTimeout(time.Second)
	quotes, err := api.GetQuote("sz000001", "hk00700")
	if err != nil || len(quotes) != 2 {
		t.Fatal(quotes, err)
	}
	f, err := api.GetFinanceInfo(protocol.ExchangeSZ, "000001")
	if err != nil || f.LiuTongGuBen != 20000 || f.ZongGuBen != 30000 {
		t.Fatal(f, err)
	}
	cats, err := api.GetCompanyCategory(protocol.ExchangeSZ, "000001")
	if err != nil || len(cats) != 1 || cats[0].Start != 7 || cats[0].Length != 4 {
		t.Fatal(cats, err)
	}
	content, err := api.GetCompanyContent(protocol.ExchangeSZ, "000001", "info.txt", 7, 4)
	if err != nil || content != "info" {
		t.Fatal(content, err)
	}
	if _, err = api.GetGbbq("sz000001"); err != nil {
		t.Fatal(err)
	}
	if _, err = api.GetCallAuction("sz000001"); err != nil {
		t.Fatal(err)
	}
	raw, err := api.GetReportFile("sample.txt")
	if err != nil || string(raw) != "data" {
		t.Fatal(raw, err)
	}
	if std.Load() != 7 || ex.Load() != 1 {
		t.Fatal("wrong routing", std.Load(), ex.Load())
	}
	if err = api.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = api.GetFinanceInfo(protocol.ExchangeSZ, "000001"); err == nil {
		t.Fatal("closed facade queried finance")
	}
	if std.Load() != 7 {
		t.Fatal("query wrote after Close")
	}
}

func TestClientInterfaceRejectsUnsupportedCapabilities(t *testing.T) {
	var requests atomic.Int32
	native := respondingTestClient(t, true, func(_, _ []byte) []byte { requests.Add(1); return nil })
	var api Client = native
	for _, call := range []func() error{
		func() error { _, e := api.GetFinanceInfo(protocol.ExchangeSZ, "000001"); return e },
		func() error { _, e := api.GetCompanyCategory(protocol.ExchangeSZ, "000001"); return e },
		func() error { _, e := api.GetCompanyContent(protocol.ExchangeSZ, "000001", "a", 0, 1); return e },
		func() error { _, e := api.GetGbbq("sz000001"); return e },
		func() error { _, e := api.GetGbbqAll(); return e },
		func() error { _, e := api.GetCallAuction("sz000001"); return e },
		func() error { _, e := api.GetStockCodeAll(); return e },
		func() error { _, e := api.GetETFCodeAll(); return e },
		func() error { _, e := api.GetIndexCodeAll(); return e },
		func() error { _, e := api.GetBlockData("a"); return e },
		func() error { _, e := api.GetBlockDataWithIndex("a"); return e },
		func() error { _, e := api.GetBlockFileRaw("a"); return e },
		func() error { _, e := api.GetReportFile("a"); return e },
		func() error { _, e := api.GetZHBFiles(); return e },
		func() error { _, e := api.GetTdxZs(); return e },
		func() error { _, e := api.GetTdxBk(); return e },
		func() error { _, e := api.GetTdxStat(); return e },
		func() error { _, e := api.GetTdxStat2(); return e },
		func() error { _, e := api.GetXgsg(); return e },
		func() error { _, e := api.GetSpBlock(); return e },
		func() error { _, e := api.GetTdxHy(); return e },
	} {
		if err := call(); err == nil {
			t.Fatal("unsupported standard capability accepted")
		}
	}
	if requests.Load() != 0 {
		t.Fatal("unsupported operation emitted a frame")
	}
	unified := NewClient()
	defer unified.Close()
	m := unified.(*MarketClient)
	var dials atomic.Int32
	m.dial[0] = func() (protocolClient, error) { dials.Add(1); return nil, io.ErrClosedPipe }
	if _, err := unified.GetFinanceInfo(protocol.ExchangeHK, "00700"); err == nil {
		t.Fatal("HK finance accepted")
	}
	if _, err := unified.GetCompanyContent(protocol.ExchangeSZ, "too-long", "a", 0, 1); err == nil {
		t.Fatal("invalid code accepted")
	}
	if _, err := unified.GetGbbq("us.AAPL"); err == nil {
		t.Fatal("US equity data accepted")
	}
	if dials.Load() != 0 {
		t.Fatal("invalid request opened a connection")
	}
	if r, err := unified.GetReportFile("a"); r != nil || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("dial failure swallowed", r, err)
	}
}

func TestClientInterfaceCloseStopsReconnection(t *testing.T) {
	native := pipeTestClient(t, false, func(s net.Conn) {
		for {
			h, _, err := readTestFrame(s)
			if err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:]) == 0 {
				continue
			}
			if writeTestResponse(s, h, []byte{1, 0}) != nil {
				return
			}
		}
	}, WithRedial(true))
	var api Client = native
	if _, err := api.GetCount(protocol.ExchangeSZ); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- api.Close() }()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("interface Close did not end lifecycle")
	}
	for j := 0; j < 8; j++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	select {
	case <-native.Runner2.Done():
	default:
		t.Fatal("Close left reconnect loop running")
	}
	if _, err := api.GetMarkets(); err == nil {
		t.Fatal("closed native interface remained usable")
	}
}

func TestMarketClientUsesProtocolImplementations(t *testing.T) {
	m := marketTestClient(t, func(_, _ []byte) []byte { return nil }, func(_, _ []byte) []byte { return nil })
	std, err := m.connection(protocol.ExchangeSH)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := std.(*testClient); !ok {
		t.Fatalf("standard route returned %T", std)
	}
	ex, err := m.connection(protocol.ExchangeHK)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ex.(*testClient); !ok {
		t.Fatalf("extended route returned %T", ex)
	}
	if std != m.links[0] || ex != m.links[1] {
		t.Fatal("facade did not reuse the connection implementation")
	}
}

func TestMarketClientRejectsWrongDialProtocol(t *testing.T) {
	for _, empty := range []bool{false, true} {
		m := newMarketClient()
		t.Cleanup(func() { m.Close() })
		wrong := respondingTestClient(t, true, func(_, _ []byte) []byte { return nil })
		m.dial[0] = func() (protocolClient, error) {
			if empty {
				return nil, nil
			}
			return wrong, nil
		}
		if _, err := m.GetCount(protocol.ExchangeSZ); err == nil || m.links[0] != nil {
			t.Fatal("invalid protocol connection was published", err)
		}
		if !empty {
			select {
			case <-wrong.Done():
			default:
				t.Fatal("rejected connection leaked")
			}
		}
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

func TestMarketClientCloseCancelsInitialRetry(t *testing.T) {
	previous, previousEx := Hosts, ExHosts
	listener := localDialListener(t)
	addr := listener.Addr().String()
	listener.Close()
	Hosts, ExHosts = Addrs{addr}, Addrs{addr}
	t.Cleanup(func() { Hosts, ExHosts = previous, previousEx })
	for _, exchange := range []protocol.Exchange{protocol.ExchangeSZ, protocol.ExchangeHK} {
		t.Run(exchange.String(), func(t *testing.T) {
			started := make(chan struct{})
			var once sync.Once
			c := newMarketClient(func(native *client.Client) {
				native.Event.OnReconnect = func(int) (time.Duration, error) {
					once.Do(func() { close(started) })
					return time.Hour, nil
				}
			}, WithDebug(false), WithLevel(LevelNone))
			queryDone := make(chan error, 1)
			go func() { _, err := c.connection(exchange); queryDone <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("initial retry did not start")
			}
			closeDone := make(chan error, 1)
			go func() { closeDone <- c.Close() }()
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("Close did not cancel initial retry")
			}
			select {
			case err := <-queryDone:
				if !errors.Is(err, net.ErrClosed) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("initial query did not return")
			}
		})
	}
}

func TestMinuteStandardResponse(t *testing.T) {
	p := &protocol.PriceNumber{Time: "09:31", Price: 12345, Number: 7}
	r := &MinuteResp{Count: 99, List: []Minute{{Standard: p}}}
	out, err := r.StandardResponse()
	if err != nil || out.Count != 1 || len(out.List) != 1 || out.List[0] != *p {
		t.Fatal(out, err)
	}
	for _, r := range []*MinuteResp{
		{Instrument: protocol.Instrument{Exchange: protocol.ExchangeHK}},
		{List: []Minute{{}}},
		{List: make([]Minute, 65536)},
	} {
		if out, err := r.StandardResponse(); out != nil || err == nil {
			t.Fatal("invalid conversion accepted", out, err)
		}
	}
	out, err = (&MinuteResp{}).StandardResponse()
	if err != nil || out.Count != 0 || len(out.List) != 0 {
		t.Fatal("empty result changed", out, err)
	}
}

func TestKlinesVolumeValidation(t *testing.T) {
	for _, tc := range []struct {
		volume, raw, want int64
		valid             bool
	}{
		{12300, 123, 123, true},   // 股票原始量为百股，统一量为股。
		{12300, 12300, 123, true}, // 指数原始量和统一量均为股。
		{0, 0, 0, true},
		{12301, 12301, 0, false},
		{-100, 0, 0, false},
	} {
		r := &KlineResp{List: []Kline{{Volume: tc.volume, Standard: &protocol.Kline{Volume: tc.raw}}}}
		ks, err := r.Klines()
		if (err == nil) != tc.valid {
			t.Fatalf("volume=%d: %v", tc.volume, err)
		}
		if tc.valid && (ks[0].Volume != tc.want || r.List[0].Standard.Volume != tc.raw || r.List[0].Volume != tc.volume) {
			t.Fatal("incorrect or mutating conversion", ks)
		}
	}
}
