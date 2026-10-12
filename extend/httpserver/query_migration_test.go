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
	open := tdx.Dial
	if extended {
		open = tdx.DialExHq
	}
	c, err := open(l.Addr().String(), tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone), tdx.WithDebug(false))
	if err != nil {
		l.Close()
		<-done
		t.Fatal(err)
	}
	p, err := tdx.NewPool(func() (*tdx.Conn, error) { return c, nil }, 1)
	if err != nil {
		c.CloseAll()
		l.Close()
		<-done
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(); l.Close(); <-done })
	return p
}

func TestUnifiedQueriesPreserveHTTPPayloads(t *testing.T) {
	s := &Server{pool: migrationPool(t, false, true), exPool: migrationPool(t, true, true)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	for _, tc := range []struct {
		url, field string
		value      float64
		list       bool
	}{
		{"/quote?codes=sz000001", "Close", 10000, true},
		{"/minute?code=sz000001", "Price", 10000, false},
		{"/minute/history?date=20261009&code=sz000001", "Price", 10000, false},
		{"/trade?code=sz000001&start=0&count=1", "Price", 10000, false},
		{"/trade/history?date=20261009&code=sz000001&start=0&count=1", "Price", 10000, false},
		{"/kline?type=9&code=sz000001&start=0&count=1", "Close", 10000, false},
		{"/ex/quote?market=31&code=00700", "price", 10, false},
		{"/ex/minute?market=31&code=00700", "price", 10, true},
		{"/ex/minute/hist?market=31&code=00700&date=20261009", "price", 10, true},
		{"/ex/trade?market=31&code=00700&start=0&count=1", "price", 10000, true},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", tc.url, nil))
			var result struct {
				Code int
				Data json.RawMessage
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 {
				t.Fatal(w.Body.String(), err)
			}
			var item map[string]any
			if tc.list {
				var list []map[string]any
				if err := json.Unmarshal(result.Data, &list); err != nil || len(list) != 1 {
					t.Fatal(string(result.Data), err)
				}
				item = list[0]
				if tc.field == "Close" {
					item = item["Kline"].(map[string]any)
				}
			} else {
				if err := json.Unmarshal(result.Data, &item); err != nil {
					t.Fatal(err)
				}
				if list, ok := item["List"].([]any); ok {
					item = list[0].(map[string]any)
				}
			}
			if item[tc.field] != tc.value {
				t.Fatalf("payload or units changed: %s", result.Data)
			}
		})
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/ex/bars?category=4&market=31&code=00700&start=0&count=1", nil))
	var result struct {
		Code int
		Data json.RawMessage
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || string(result.Data) != "[]" {
		t.Fatal("empty extended list changed", w.Body.String(), err)
	}
}

func TestHistoryTradeHTTPWithoutDailyKline(t *testing.T) {
	s := &Server{exPool: migrationPool(t, true, false)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/ex/trade/hist?market=31&code=00700&date=20261009&start=0&count=1", nil))
	var result struct {
		Code int
		Data json.RawMessage
	}
	// 模拟服务器没有日线，但分笔接口非空，HTTP 应返回空列表。
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || string(result.Data) != "[]" {
		t.Fatal(w.Body.String(), err)
	}
}

func TestUnifiedCodesPreserveHTTPPayloads(t *testing.T) {
	s := &Server{pool: migrationPool(t, false, true), exPool: migrationPool(t, true, true)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	for _, tc := range []struct{ url, want string }{
		{"/count?exchange=sz", `{"Count":1}`},
		{"/code?exchange=sz&start=0", `{"Count":1,"List":[{"Name":"NAME","Code":"000001","Multiple":100,"Decimal":4,"LastPrice":0}]}`},
		{"/code/all?exchange=sz", `{"Count":1,"List":[{"Name":"NAME","Code":"000001","Multiple":100,"Decimal":4,"LastPrice":0}]}`},
		{"/code?exchange=sz&start=1", `{"Count":0,"List":null}`},
		{"/ex/count", `3`},
		{"/ex/instruments?start=1&count=1", `[{"category":13,"market":74,"code":"AAPL","name":"Apple","desc":"desc"}]`},
		{"/ex/markets", `[{"market":74,"category":13,"name":"US","shortName":"US"}]`},
	} {
		t.Run(tc.url, func(t *testing.T) {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", tc.url, nil))
			var result struct {
				Code int
				Data json.RawMessage
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || string(result.Data) != tc.want {
				t.Fatal(w.Body.String(), err)
			}
		})
	}
}

func TestUnifiedConveniencePreserveHTTPPayloads(t *testing.T) {
	s := &Server{pool: migrationPool(t, false, true), exPool: migrationPool(t, true, true)}
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	paths := []string{"/kline/all?type=9", "/index?type=9", "/index/all?type=9"}
	for _, period := range []string{"minute", "5minute", "15minute", "30minute", "60minute", "day", "week", "month", "quarter", "year"} {
		paths = append(paths, "/kline/"+period+"?", "/kline/"+period+"/all?")
		if period == "week" || period == "month" || period == "quarter" || period == "year" {
			paths = append(paths, "/index/"+period+"/all?")
		} else {
			paths = append(paths, "/index/"+period+"?")
		}
	}
	paths = append(paths, "/index/day/all?")
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			code := "sz000001"
			if len(path) >= 6 && path[:6] == "/index" {
				code = "sh000001"
			}
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", path+"&code="+code+"&start=0&count=1", nil))
			var result struct {
				Code int
				Data struct {
					Count int
					List  []map[string]any
				}
			}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data.Count != 1 || result.Data.List[0]["Close"] != float64(10000) {
				t.Fatal(w.Body.String(), err)
			}
			if code == "sh000001" && result.Data.List[0]["UpCount"] != float64(3) {
				t.Fatal("index fields lost", w.Body.String())
			}
		})
	}
	for _, path := range []string{"/trade/all?code=sz000001", "/trade/history/day?date=20261009&code=sz000001"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var result struct {
			Code int
			Data struct {
				Count int
				List  []map[string]any
			}
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data.Count != 1 || result.Data.List[0]["Price"] != float64(10000) {
			t.Fatal(w.Body.String(), err)
		}
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/ex/bars/range?market=31&code=00700&date=20261009&date2=20261009", nil))
	var result struct {
		Code int
		Data json.RawMessage
	}
	want := `[{"datetime":"2026-10-09 09:31","open":10,"high":10,"low":10,"close":10,"position":123,"trade":456,"settlementPrice":9}]`
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || string(result.Data) != want {
		t.Fatal(w.Body.String(), err)
	}
}
