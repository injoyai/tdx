package httpserver

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func migrationPool(t *testing.T, extended, hasDaily bool) *tdx.Pool {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			h := make([]byte, 12)
			if _, err := io.ReadFull(conn, h); err != nil {
				return
			}
			b := make([]byte, int(binary.LittleEndian.Uint16(h[6:8]))-2)
			if _, err := io.ReadFull(conn, b); err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:5]) == 0 {
				continue
			}
			typ := binary.LittleEndian.Uint16(h[10:12])
			var data []byte
			switch typ {
			case protocol.TypeCount:
				data = []byte{1, 0}
			case protocol.TypeCode:
				data = []byte{0, 0}
				if binary.LittleEndian.Uint16(b[2:]) == 0 {
					data = make([]byte, 31)
					data[0] = 1
					copy(data[2:], "000001")
					binary.LittleEndian.PutUint16(data[8:], 100)
					copy(data[10:], "NAME")
					data[22] = 4
				}
			case protocol.TypeExCount:
				data = make([]byte, 23)
				binary.LittleEndian.PutUint32(data[19:], 3)
			case protocol.TypeExInstrument:
				data = make([]byte, 70)
				data[4] = 1
				data[6], data[7] = 13, 74
				copy(data[11:], "AAPL")
				copy(data[20:], "Apple")
				copy(data[37:], "desc")
			case protocol.TypeExMarkets:
				data = make([]byte, 66)
				data[0], data[2], data[35] = 1, 13, 74
				copy(data[3:], "US")
				copy(data[36:], "US")
			case protocol.TypeQuote:
				data = []byte{1, 0x36, 1, 0, 0, '0', '0', '0', '0', '0', '1', 0, 0, 0xa8, 0x0f}
				data = append(data, make([]byte, 46)...)
			case protocol.TypeHistoryMinute:
				data = []byte{1, 0, 0, 0, 0, 0, 0xa8, 0x0f, 0, 2}
			case protocol.TypeTrade:
				data = []byte{1, 0, 0x3a, 2, 0xa8, 0x0f, 1, 3, 1, 0}
			case protocol.TypeHistoryTrade:
				data = []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 1, 0}
			case protocol.TypeKline:
				data = []byte{1, 0}
				if b[8] <= 3 || b[8] == 7 || b[8] == 8 {
					data = append(data, protocol.Bytes(uint16((2026-2004)*2048+1000+9))...)
					data = append(data, protocol.Bytes(uint16(571))...)
				} else {
					data = append(data, protocol.Bytes(uint32(20261009))...)
				}
				data = append(data, 0x90, 0x9c, 1, 0, 0, 0)
				data = append(data, protocol.Bytes(math.Float32bits(100))...)
				data = append(data, protocol.Bytes(math.Float32bits(1000))...)
				if b[0] == 1 {
					data = append(data, 3, 0, 2, 0)
				}
			case protocol.TypeExQuote:
				data = make([]byte, 150)
				copy(data, b)
				binary.LittleEndian.PutUint32(data[30:34], math.Float32bits(10))
			case protocol.TypeExBarsRange:
				data = make([]byte, 46)
				binary.LittleEndian.PutUint16(data[12:], 1)
				binary.LittleEndian.PutUint16(data[14:], uint16((2026-2004)*2048+1000+9))
				binary.LittleEndian.PutUint16(data[16:], 571)
				for p := 18; p < 34; p += 4 {
					binary.LittleEndian.PutUint32(data[p:], math.Float32bits(10))
				}
				binary.LittleEndian.PutUint32(data[34:], 123)
				binary.LittleEndian.PutUint32(data[38:], 456)
				binary.LittleEndian.PutUint32(data[42:], math.Float32bits(9))
			case protocol.TypeExBars:
				data = make([]byte, 20) // 合法空列表；原 HTTP 响应必须仍为 []。
				if hasDaily && binary.LittleEndian.Uint16(b[18:20]) == 800 {
					// 历史分笔的交易日检查需要对应日线；直接单根查询仍为空。
					data = make([]byte, 52)
					binary.LittleEndian.PutUint16(data[18:], 1)
					binary.LittleEndian.PutUint32(data[20:], 20261009)
				}
			case protocol.TypeExMinute, protocol.TypeExHistMinute:
				header := 12
				if typ == protocol.TypeExHistMinute {
					header = 20
				}
				data = make([]byte, header+18)
				binary.LittleEndian.PutUint16(data[header-2:header], 1)
				binary.LittleEndian.PutUint16(data[header:header+2], 570)
				binary.LittleEndian.PutUint32(data[header+2:header+6], math.Float32bits(10))
			case protocol.TypeExTrade, protocol.TypeExHistTrade:
				data = make([]byte, 32)
				binary.LittleEndian.PutUint16(data[14:16], 1)
				binary.LittleEndian.PutUint16(data[16:18], 570)
				binary.LittleEndian.PutUint32(data[18:22], 10000)
				binary.LittleEndian.PutUint16(data[30:32], 256)
			default:
				return
			}
			r := make([]byte, 16+len(data))
			copy(r, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
			copy(r[5:9], h[1:5])
			copy(r[10:12], h[10:12])
			binary.LittleEndian.PutUint16(r[12:14], uint16(len(data)))
			binary.LittleEndian.PutUint16(r[14:16], uint16(len(data)))
			copy(r[16:], data)
			if _, err := conn.Write(r); err != nil {
				return
			}
		}
	}()
	var c tdx.Client
	if extended {
		c, err = tdx.DialEx(l.Addr().String(), tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone), tdx.WithDebug(false))
	} else {
		c, err = tdx.Dial(l.Addr().String(), tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone), tdx.WithDebug(false))
	}
	if err != nil {
		l.Close()
		<-done
		t.Fatal(err)
	}
	p, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
	if err != nil {
		c.Close()
		l.Close()
		<-done
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(); l.Close(); <-done })
	return p
}

func httpData[T any](t *testing.T, mux *http.ServeMux, path string) T {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	var result struct {
		Code int
		Msg  string
		Data T
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || w.Code != 200 {
		t.Fatalf("%s: %d %s (%v)", path, w.Code, w.Body, err)
	}
	return result.Data
}

func TestUnifiedHTTPQueries(t *testing.T) {
	s := &Server{pool: migrationPool(t, false, true), exPool: migrationPool(t, true, true)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	quotes := httpData[[]tdx.Quote](t, mux, "/quote?codes=hk00700,sz000001,hk00700")
	if len(quotes) != 3 || quotes[0].Price != 10 || quotes[0].Extended == nil || quotes[1].Price != 10 || quotes[1].Standard.Kline.Close != 10000 || quotes[2].Instrument != quotes[0].Instrument {
		t.Fatal("mixed quotes lost order, price or raw data", quotes)
	}
	for _, code := range []string{"sz000001", "hk00700"} {
		for _, path := range []string{"/minute?", "/minute/history?date=20261009&"} {
			r := httpData[tdx.MinuteResp](t, mux, path+"code="+code)
			if r.Count != 1 || len(r.List) != 1 || r.List[0].Price != 10 || r.VolumeUnit == "" {
				t.Fatal(path, r)
			}
		}
		for _, path := range []string{"/trade?", "/trade/all?", "/trade/history?date=20261009&", "/trade/history/day?date=20261009&"} {
			r := httpData[tdx.TradeResp](t, mux, path+"code="+code+"&start=0&count=1")
			if r.Count != 1 || len(r.List) != 1 || r.VolumeUnit == "" {
				t.Fatal(path, r)
			}
			row := r.List[0]
			if code == "sz000001" {
				if row.Price == nil || *row.Price != 10 || row.Standard.Price != 10000 {
					t.Fatal(row)
				}
			} else if row.Price != nil || row.Extended.Price != 10000 {
				t.Fatal("unknown extended price scale was guessed", row)
			}
		}
	}
	empty := httpData[tdx.KlineResp](t, mux, "/kline/day?code=hk00700&start=0&count=1")
	if empty.Count != 0 || len(empty.List) != 0 {
		t.Fatal(empty)
	}
	r := httpData[tdx.KlineResp](t, mux, "/kline/range?code=hk00700&date=20261009&date2=20261009")
	if r.Count != 1 || r.List[0].Close != 10 || r.List[0].RangeExtended.Position != 123 || r.List[0].RangeExtended.Trade != 456 || r.List[0].RangeExtended.SettlementPrice != 9 {
		t.Fatal(r)
	}
}

func TestHistoryTradeHTTPWithoutDailyKline(t *testing.T) {
	s := &Server{exPool: migrationPool(t, true, false)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	r := httpData[tdx.TradeResp](t, mux, "/trade/history?code=hk00700&date=20261009&start=0&count=1")
	if r.Count != 0 || len(r.List) != 0 {
		t.Fatal("history without a daily kline was accepted", r)
	}
}

func TestUnifiedHTTPCodes(t *testing.T) {
	s := &Server{pool: migrationPool(t, false, true), exPool: migrationPool(t, true, true)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	if n := httpData[int](t, mux, "/count?exchange=sz"); n != 1 {
		t.Fatal(n)
	}
	for _, path := range []string{"/code?exchange=sz&start=0&count=1", "/code/all?exchange=sz", "/code?exchange=us&start=0&count=1", "/code/all?exchange=us"} {
		r := httpData[tdx.CodeResp](t, mux, path)
		want := 1
		if path == "/code/all?exchange=us" {
			want = 3
		}
		if r.Count != want || len(r.List) != want || r.List[0].Instrument.Code == "" {
			t.Fatal(path, r)
		}
	}
	r := httpData[tdx.CodeResp](t, mux, "/code?exchange=sz&start=1&count=1")
	if r.Count != 0 || len(r.List) != 0 {
		t.Fatal(r)
	}
	markets := httpData[[]tdx.Exchange](t, mux, "/markets")
	if len(markets) != 4 || markets[0].Protocol != protocol.ProtocolStandard || markets[3].Extended.Market != 74 {
		t.Fatal(markets)
	}
	// These raw endpoints expose distinct global discovery, rather than per-market queries.
	if n := httpData[int](t, mux, "/ex/count"); n != 3 {
		t.Fatal(n)
	}
	raw := httpData[[]protocol.ExInstrument](t, mux, "/ex/instruments?start=1&count=1")
	if len(raw) != 1 || raw[0].Code != "AAPL" {
		t.Fatal(raw)
	}
}

func TestUnifiedHTTPKlinePeriods(t *testing.T) {
	s := &Server{pool: migrationPool(t, false, true)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	paths := []string{"/kline?type=9&", "/kline/all?type=9&"}
	for _, period := range []string{"minute", "5minute", "15minute", "30minute", "60minute", "day", "week", "month", "quarter", "year"} {
		paths = append(paths, "/kline/"+period+"?", "/kline/"+period+"/all?")
	}
	for _, code := range []string{"sz000001", "sh000001"} {
		for _, path := range paths {
			r := httpData[tdx.KlineResp](t, mux, path+"code="+code+"&start=0&count=1")
			if r.Count != 1 || len(r.List) != 1 || r.List[0].Close != 10 || r.List[0].Standard.Close != 10000 {
				t.Fatal(path, r)
			}
			if code == "sh000001" && r.List[0].Standard.UpCount != 3 {
				t.Fatal("index fields lost", r)
			}
		}
	}
}

func TestRemovedHTTPAliases(t *testing.T) {
	mux := http.NewServeMux()
	(&Server{}).registerRoutes(mux)
	for _, path := range []string{"/index", "/index/day", "/index/day/all", "/ex/quote", "/ex/bars", "/ex/minute", "/ex/minute/hist", "/ex/trade", "/ex/trade/hist", "/ex/bars/range", "/ex/markets", "/all", "/unknown"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNotFound {
			t.Fatal(path, w.Code, w.Body)
		}
	}
}
