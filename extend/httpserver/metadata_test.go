package httpserver

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
)

func TestCodesHandlerUnavailableSnapshot(t *testing.T) {
	var empty *metadata.Codes
	for _, source := range []CodesSource{nil, empty} {
		w := httptest.NewRecorder()
		CodesHandler(source, nil).ServeHTTP(w, httptest.NewRequest("GET", "/codes", nil))
		if w.Code != 503 {
			t.Fatal("missing snapshot did not return 503", w.Code)
		}
	}
}

func TestCodesHandlerRoutes(t *testing.T) {
	c, err := metadata.NewCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	stock := &model.CodeModel{Exchange: "sh", Code: "600000", Name: "stock"}
	etf := &model.CodeModel{Exchange: "sh", Code: "510300", Name: "etf"}
	index := &model.CodeModel{Exchange: "sh", Code: "000001", Name: "index"}
	if err := c.Update(model.CodeModels{index, etf, stock}); err != nil {
		t.Error(err)
	}
	for _, equity := range []bool{false, true} {
		var g *metadata.Gbbq
		if equity {
			g = &metadata.Gbbq{}
		}
		h := CodesHandler(c, g)
		for _, tc := range []struct {
			path string
			want model.CodeModels
		}{
			{"/codes", model.CodeModels{index, etf, stock}},
			{"/stocks", model.CodeModels{stock}}, {"/etfs", model.CodeModels{etf}}, {"/indexes", model.CodeModels{index}},
		} {
			for _, method := range []string{"GET", "POST"} {
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest(method, tc.path+"?v=1", nil))
				var got model.CodeModels
				if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" {
					t.Fatal(tc.path, w.Code, w.Header())
				}
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, tc.want) {
					t.Fatal("raw JSON contract changed", tc.path, w.Body.String(), err)
				}
			}
		}
		for _, path := range []string{"/gbbqs?v=1", "/missing", "/all"} {
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
