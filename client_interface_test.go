package tdx

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

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
	var cs Codes
	WithCodesClient(api)(&cs)
	var g Gbbq
	WithGbbqClient(api)(&g)
	var w Workday
	WithWorkdayClient(api)(&w)
	if cs.c != api || g.c != api || w.c != api {
		t.Fatal("metadata manager requires a concrete connection")
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

func TestClientInterfaceNilInjection(t *testing.T) {
	if reflect.TypeOf((*Client)(nil)).Elem().Kind() != reflect.Interface {
		t.Fatal("Client is not an interface")
	}
	for _, api := range []Client{(*StConn)(nil), (*MarketClient)(nil), nil} {
		var cs Codes
		WithCodesClient(api)(&cs)
		var g Gbbq
		WithGbbqClient(api)(&g)
		var w Workday
		WithWorkdayClient(api)(&w)
		if cs.c != nil || g.c != nil || w.c != nil {
			t.Fatal("typed nil client no longer follows nil injection behavior")
		}
	}
	dial := func() (Client, error) { return (*StConn)(nil), nil }
	if cs, err := NewCodes(WithCodesDialClient(dial)); cs != nil || err == nil {
		t.Fatal("nil factory accepted", cs, err)
	}
	if g, err := NewGbbq(WithGbbqDialClient(dial)); g != nil || err == nil {
		t.Fatal("nil factory accepted", g, err)
	}
}
