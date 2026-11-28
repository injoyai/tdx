package tdx_test

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

const (
	testCash protocol.MarketID = "testex/cash"
	testFund protocol.MarketID = "testex/fund"
)

type testMarket struct{ info protocol.MarketInfo }

var _ protocol.Market = (*testMarket)(nil)

func (m *testMarket) Info() protocol.MarketInfo { return m.info }

func (*testMarket) Resolve(c protocol.MarketCode, _ protocol.InstrumentSnapshot) (protocol.Instrument, error) {
	var ex protocol.Exchange
	switch c.Symbol {
	case "Aa12", "Cc56":
		ex = 252
	case "Bb34":
		ex = 251
	default:
		return protocol.Instrument{}, fmt.Errorf("unexpected code %q", c.Symbol)
	}
	if c.Explicit() && c.Exchange != ex {
		return protocol.Instrument{}, fmt.Errorf("code %q belongs to %d", c.Symbol, ex)
	}
	return protocol.Instrument{Exchange: ex, Code: c.Symbol}, nil
}

func (*testMarket) Rules(i protocol.Instrument) (protocol.InstrumentRules, error) {
	r := protocol.ExtendedRules(protocol.SecurityStock)
	r.KlinePeriods = 1<<protocol.TypeKlineDay2 | 1<<protocol.TypeKlineMinute2
	r.RangePeriods = 0
	r.Day.VolumeScale, r.Minute.VolumeScale = 10, 2
	r.DayAmountScale, r.MinuteAmountScale = 100, 20
	r.DayFields = protocol.Fields{Price: true, Volume: true, Amount: true}
	r.IntradayFields = r.DayFields
	if i.Exchange == 251 {
		r.Security = protocol.SecurityFund
		r.DayAmountScale, r.MinuteAmountScale = 4, 8
	}
	return r, nil
}

func TestApplicationMarketRegistration(t *testing.T) {
	const rejected protocol.MarketID = "rejectex"
	m := &testMarket{info: protocol.MarketInfo{
		Prefix: "testex", Name: "Test Exchange", TimeZone: "UTC", CloseTime: "15:00", Version: "1",
		Routes: []protocol.RouteInfo{
			{ID: testCash, Exchange: 252, Protocol: protocol.ProtocolExtended, CodeSource: protocol.CodeListExtended, BareSymbols: []string{"Aa12"}},
			{ID: testFund, Exchange: 251, Protocol: protocol.ProtocolExtended, CodeSource: protocol.CodeListExtended},
		},
	}}
	if _, known := testCash.Exchange(); !known {
		bad := &testMarket{info: protocol.MarketInfo{Prefix: rejected.String(), Routes: []protocol.RouteInfo{
			{ID: rejected, Exchange: 252, Protocol: protocol.ProtocolExtended, CodeSource: protocol.CodeListExtended, BareSymbols: []string{"Aa12"}},
			{ID: "rejectex/other", Exchange: protocol.ExchangeCSI, Protocol: protocol.ProtocolExtended, CodeSource: protocol.CodeListExtended},
		}}}
		if err := protocol.Register(bad); err == nil {
			t.Fatal("registration with a conflicting second route succeeded")
		}
		if _, ok := pull.Get(rejected.String()); ok || slices.Contains(protocol.Markets(), rejected) {
			t.Fatal("failed registration published a route")
		}
		if _, ok := protocol.Exchange(252).Route(); ok {
			t.Fatal("failed registration published a wire number")
		}
		// This also exercises reuse of the first route's wire number and bare symbol.
		if err := protocol.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := protocol.Register(m); err == nil {
		t.Fatal("duplicate market object accepted")
	}
	for _, tc := range []struct {
		input string
		id    protocol.MarketID
		ex    protocol.Exchange
		code  string
	}{
		{"TESTEX.Aa12", testCash, 252, "Aa12"},
		{"Aa12", testCash, 252, "Aa12"},
		{"testexBb34", testFund, 251, "Bb34"},
		{"Bb34.TESTEX", testFund, 251, "Bb34"},
		{"testex/fund.Bb34", testFund, 251, "Bb34"},
		{"Bb34.251", testFund, 251, "Bb34"},
	} {
		i, err := protocol.ParseInstrument(tc.input)
		if err != nil || i.Exchange != tc.ex || i.Code != tc.code {
			t.Fatal(tc.input, i, err)
		}
		code, err := pull.ParseCode(tc.input)
		if err != nil || code.Market != tc.id || code.Code != tc.code {
			t.Fatal(tc.input, code, err)
		}
	}
	if _, err := pull.ParseCode("testex/cash.Bb34"); err == nil {
		t.Fatal("explicit route accepted an instrument from another route")
	}

	units := make(map[string]int)
	for _, u := range pull.Units() {
		units[u.Name()]++
	}
	for _, id := range []protocol.MarketID{testCash, testFund, protocol.MarketHK, protocol.MarketHKFund, protocol.MarketHKIndex, protocol.MarketCSI} {
		u, ok := pull.Get(strings.ToUpper(id.String()))
		if !ok || u.Name() != id.String() || units[id.String()] != 1 {
			t.Fatalf("inconsistent route view for %s", id)
		}
	}

	source := newMarketFixture(t)
	for _, tc := range []struct {
		input string
		id    protocol.MarketID
		day   float64
		min   float64
	}{
		{"Aa12.TESTEX", testCash, 125, 25},
		{"testex.Bb34", testFund, 5, 10},
	} {
		for _, period := range []struct {
			category uint8
			at       string
			volume   int64
			amount   float64
		}{
			{protocol.TypeKlineDay, "2026-01-05", 4560, tc.day},
			{protocol.TypeKlineMinute, "2026-01-05 09:31", 912, tc.min},
		} {
			resp, err := source.ex.GetKline(period.category, tc.input, 0, 1)
			if err != nil || resp.Count != 1 || resp.List[0].Time != period.at ||
				resp.List[0].Volume != period.volume || resp.List[0].Amount == nil || *resp.List[0].Amount != period.amount {
				t.Fatal(tc.input, period, resp, err)
			}
			if id, err := resp.Instrument.Market(); err != nil || id != tc.id {
				t.Fatal(id, err)
			}
		}
	}

	for _, mode := range []string{"discover", "explicit"} {
		t.Run(mode, func(t *testing.T) {
			// Keep SQLite fixtures under the project's ignored output directory.
			if err := os.MkdirAll("output", 0o755); err != nil {
				t.Fatal(err)
			}
			dir, err := os.MkdirTemp("output", "test-market-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			store, err := sqlite.NewPull(dir, 4)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			cfg := pull.Config{Store: store, ExPool: source, StartAt: "20260101", Goroutines: 1}
			if mode == "explicit" {
				cfg.Codes = []string{"Aa12.TESTEX", "testex.Bb34"}
				cfg.Schedules = map[protocol.MarketID]pull.Schedule{testCash: {TimeZone: "Asia/Shanghai", CloseTime: "12:00"}}
			}
			s, err := pull.New(&cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			before := source.discoveries.Load()
			if err := s.Update(true); err != nil {
				t.Fatal(err)
			}
			wantReads := int32(0)
			if mode == "discover" {
				wantReads = 1
			}
			if got := source.discoveries.Load() - before; got != wantReads {
				t.Fatalf("discovery reads = %d, want %d", got, wantReads)
			}
			requireStoredBars(t, store, model.Code{Market: testCash, Code: "Aa12"}, 4560, 912, 125, 25)
			requireStoredBars(t, store, model.Code{Market: testFund, Code: "Bb34"}, 4560, 912, 5, 10)
			if mode == "discover" {
				requireStoredBars(t, store, model.Code{Market: testCash, Code: "Cc56"}, 4560, 912, 125, 25)
				requireStoredBars(t, store, model.Code{Market: protocol.MarketHK, Code: "00700"}, 45600, 45600, 1.25, 1.25)
				requireStoredBars(t, store, model.Code{Market: protocol.MarketHKFund, Code: "02800"}, 45600, 45600, 1.25, 1.25)
				requireStoredBars(t, store, model.Code{Market: protocol.MarketHKIndex, Code: "HSI"}, 0, 0, 1250000, 1250000)
				requireStoredBars(t, store, model.Code{Market: protocol.MarketCSI, Code: "931079"}, 456000000, 456000000, 1250000, 1250000)
			} else if rows, err := store.QueryDay(model.Code{Market: testCash, Code: "Cc56"}, "", ""); err != nil || len(rows) != 0 {
				t.Fatal("whitelist included a discovered code", rows, err)
			}
		})
	}
}

func requireStoredBars(t *testing.T, s *sqlite.PullStore, code model.Code, dayVolume, minuteVolume int64, dayAmount, minuteAmount float64) {
	t.Helper()
	days, err := s.QueryDay(code, "", "")
	if err != nil || len(days) != 1 || days[0].Time != "2026-01-05" || days[0].Volume != dayVolume ||
		days[0].Amount == nil || *days[0].Amount != dayAmount {
		t.Fatal(code, days, err)
	}
	minutes, err := s.QueryMinute(code, "", "")
	if err != nil || len(minutes) != 1 || minutes[0].Time != "2026-01-05 09:31" || minutes[0].Volume != minuteVolume ||
		minutes[0].Amount == nil || *minutes[0].Amount != minuteAmount {
		t.Fatal(code, minutes, err)
	}
	if _, err := os.Stat(s.DayFile(code)); err != nil {
		t.Fatal(code, err)
	}
	if _, err := os.Stat(s.MinuteFile(code, 2026)); err != nil {
		t.Fatal(code, err)
	}
	if got := filepath.Dir(filepath.Dir(s.DayFile(code))); got != filepath.Join(s.Identity(), code.Market.String()) {
		t.Fatal(code, "wrong storage partition", got)
	}
}

type marketFixture struct {
	ex          *tdx.ExConn
	discoveries atomic.Int32
}

func (s *marketFixture) DoContext(ctx context.Context, f func(tdx.Client) error) error {
	return f(tdx.WithContext(ctx, s.ex))
}

func newMarketFixture(t *testing.T) *marketFixture {
	t.Helper()
	s := &marketFixture{}
	// Fixed 7727 payloads: one bar, OHLC 10.25/11.25/9.25/10.5, quantity 456, amount 1.25.
	day, _ := hex.DecodeString("000000000000000000000000000000000000010009253501000024410000344100001441000028410000a03fc801000000002c41")
	minute, _ := hex.DecodeString("000000000000000000000000000000000000010069b03b02000024410000344100001441000028410000a03fc801000000002c41")
	items := []struct {
		ex   protocol.Exchange
		code string
	}{
		{252, "Aa12"}, {252, "Cc56"}, {251, "Bb34"},
		{protocol.ExchangeHK, "00700"}, {protocol.ExchangeHKFund, "02800"}, {protocol.ExchangeHI, "HSI"},
		{protocol.ExchangeCSI, "931079"},
	}
	a, b := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer b.Close()
		for {
			var header [12]byte
			if _, err := io.ReadFull(b, header[:]); err != nil {
				return
			}
			body := make([]byte, int(binary.LittleEndian.Uint16(header[6:8]))-2)
			if _, err := io.ReadFull(b, body); err != nil {
				return
			}
			if binary.LittleEndian.Uint32(header[1:5]) == 0 {
				continue
			}
			var payload []byte
			switch typ := binary.LittleEndian.Uint16(header[10:12]); typ {
			case protocol.TypeExCount:
				s.discoveries.Add(1)
				payload = make([]byte, 23)
				binary.LittleEndian.PutUint32(payload[19:], uint32(len(items)))
			case protocol.TypeExInstrument:
				payload = make([]byte, 6+64*len(items))
				binary.LittleEndian.PutUint16(payload[4:], uint16(len(items)))
				for j, item := range items {
					p := 6 + j*64
					payload[p+1] = item.ex.Uint8()
					copy(payload[p+5:p+14], item.code)
				}
			case protocol.TypeExBars:
				symbol := strings.TrimRight(string(body[1:10]), "\x00")
				if !slices.ContainsFunc(items, func(item struct {
					ex   protocol.Exchange
					code string
				}) bool {
					return item.ex.Uint8() == body[0] && item.code == symbol
				}) {
					t.Errorf("unexpected bar route %d/%s", body[0], symbol)
					return
				}
				switch {
				case binary.LittleEndian.Uint32(body[14:18]) != 0:
					payload = make([]byte, 20)
				case body[10] == protocol.TypeKlineDay2:
					payload = day
				case body[10] == protocol.TypeKlineMinute2:
					payload = minute
				default:
					t.Errorf("unexpected wire category %d", body[10])
					return
				}
			default:
				t.Errorf("unexpected request %x", typ)
				return
			}
			response := make([]byte, 16+len(payload))
			copy(response, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
			copy(response[5:9], header[1:5])
			copy(response[10:12], header[10:12])
			binary.LittleEndian.PutUint16(response[12:14], uint16(len(payload)))
			binary.LittleEndian.PutUint16(response[14:16], uint16(len(payload)))
			copy(response[16:], payload)
			if _, err := b.Write(response); err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { a.Close(); b.Close(); <-done })
	ex, err := tdx.DialExWith(func(context.Context) (ios.ReadWriteCloser, string, error) {
		return a, "offline-market", nil
	}, tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone))
	if err != nil {
		t.Fatal(err)
	}
	ex.SetTimeout(time.Second)
	t.Cleanup(func() { ex.Close() })
	s.ex = ex
	return s
}
