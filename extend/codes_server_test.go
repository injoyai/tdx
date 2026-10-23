package extend

import (
	"encoding/json"
	"errors"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCodesHTTPStartupErrors(t *testing.T) {
	want := errors.New("offline initialization failure")
	options := []codes.Option{codes.WithDialClient(func() (tdx.Client, error) { return nil, want })}
	for _, test := range []struct {
		name string
		fn   func() error
	}{
		{"codes", func() error { return ListenCodesHTTP(0, options...) }},
		{"codes and equity", func() error { return ListenCodesAndGbbqHTTP(0, options, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); !errors.Is(err, want) {
				t.Errorf("initialization failure reported as success: got=%v", err)
			}
		})
	}
}

func TestGbbqHTTPEffectiveDay(t *testing.T) {
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	old := &protocol.Gbbq{Code: "sz000001", Time: day.Add(-9 * time.Hour), Category: 5, C3: 100, C4: 200}
	current := &protocol.Gbbq{Code: "sz000001", Time: day.Add(15 * time.Hour), Category: 5, C3: 300, C4: 400}
	g := &GbbqHTTP{m: map[string][]*protocol.Gbbq{"sz000001": {old, current}}}
	for _, tc := range []struct {
		at   time.Time
		want int64
	}{
		{day.Add(-time.Minute), 100}, {day, 300}, {day.Add(9*time.Hour + 30*time.Minute), 300}, {current.Time, 300},
	} {
		if got := g.GetEquity("sz000001", tc.at); got == nil || got.Float != tc.want {
			t.Fatalf("equity at %s = %+v, want %d", tc.at, got, tc.want)
		}
	}
}

func TestGbbqHTTPCodeForms(t *testing.T) {
	at := time.Date(2026, 10, 19, 0, 0, 0, 0, protocol.Location())
	g := &GbbqHTTP{m: map[string][]*protocol.Gbbq{"sz000001": {{Time: at, Category: 5, C3: 100, C4: 200}}}}
	for _, code := range []string{"sz000001", "SZ000001", "sz.000001", "000001.SZ", "000001"} {
		if got := g.GetEquity(code, at); got == nil || got.Float != 100 {
			t.Fatalf("%s: %+v", code, got)
		}
	}
	for _, code := range []string{"hk00001", "invalid", ""} {
		if got := g.GetEquity(code, at); got != nil {
			t.Fatalf("invalid code %s: %+v", code, got)
		}
	}
}

func TestCodesHandlerRoutes(t *testing.T) {
	c := codes.NewBase()
	stock := &codes.CodeModel{Exchange: "sh", Code: "600000", Name: "stock"}
	etf := &codes.CodeModel{Exchange: "sh", Code: "510300", Name: "etf"}
	index := &codes.CodeModel{Exchange: "sh", Code: "000001", Name: "index"}
	c.Update(codes.CodeModels{index, etf, stock})
	for _, equity := range []bool{false, true} {
		var g *gbbq.Gbbq
		if equity {
			g = &gbbq.Gbbq{}
		}
		h := codesHandler(c, g)
		for _, tc := range []struct {
			path string
			want codes.CodeModels
		}{
			{"/codes", codes.CodeModels{stock, etf, index}}, {"/all", codes.CodeModels{stock, etf, index}},
			{"/stocks", codes.CodeModels{stock}}, {"/etfs", codes.CodeModels{etf}}, {"/indexes", codes.CodeModels{index}},
		} {
			for _, method := range []string{"GET", "POST"} {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest(method, tc.path+"?v=1", nil))
				var got codes.CodeModels
				if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
					t.Fatal(tc.path, w.Code, w.Header())
				}
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Fatal("raw JSON contract changed", tc.path, w.Body.String(), err)
				}
			}
		}
		for _, path := range []string{"/gbbqs?v=1", "/missing"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			want := 404
			if equity && path == "/gbbqs?v=1" {
				want = 200
				if w.Body.String() != "{}" {
					t.Fatal(w.Body.String())
				}
			}
			if w.Code != want {
				t.Fatal(path, w.Code, want)
			}
		}
	}
}
