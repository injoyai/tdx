package httpserver

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
)

type replacingCodes struct {
	*metadata.Codes
	calls int
}

func (s *replacingCodes) Snapshot() model.CodeModels {
	s.calls++
	rows := s.Codes.Snapshot()
	fresh := s.Codes.Snapshot()
	for _, row := range fresh {
		row.Name = "next"
	}
	if err := s.Codes.Update(fresh); err != nil {
		panic(err)
	}
	return rows
}

func TestCodesHandlerUsesOneCompleteSnapshot(t *testing.T) {
	rows := model.CodeModels{
		{Exchange: "sh", Code: "600000", Name: "first"},
		{Exchange: "sh", Code: "510300", Name: "first"},
		{Exchange: "sh", Code: "000001", Name: "first"},
		{Exchange: "sh", Code: "113001", Name: "first"},
		{Exchange: "sh", Code: "019001", Name: "first"},
	}
	index, err := metadata.NewCodes(rows)
	if err != nil {
		t.Fatal(err)
	}
	s := &replacingCodes{Codes: index}
	h := CodesHandler(s, nil)
	for i, name := range []string{"first", "next"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/codes", nil))
		var got model.CodeModels
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if w.Code != 200 || len(got) != len(rows) || s.calls != i+1 {
			t.Fatal("incomplete snapshot or multiple source reads", w.Code, len(got), s.calls)
		}
		for j, row := range got {
			if row.FullCode() != rows[j].FullCode() || row.Name != name {
				t.Fatal("snapshot lost order or mixed versions", got)
			}
		}
	}
}
