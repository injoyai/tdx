package metadata

import (
	"reflect"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestCodesRejectInvalidReplacement(t *testing.T) {
	valid := model.CodeModels{{Exchange: "sz", Code: "000001", Name: "old"}}
	c, err := NewCodes(valid)
	if err != nil {
		t.Fatal(err)
	}
	for _, rows := range []model.CodeModels{
		{nil},
		{{Exchange: "SZ", Code: "000001"}},
		{{Exchange: "sz", Code: "bad"}},
		{{Exchange: "sz", Code: "000001"}, {Exchange: "sz", Code: "000001"}},
		{{Exchange: "sh", Code: "600000", Name: "new"}, nil},
	} {
		if got, err := NewCodes(rows); err == nil || got != nil {
			t.Fatal("invalid constructor input accepted", rows)
		}
		if err := c.Update(rows); err == nil {
			t.Fatal("invalid replacement accepted", rows)
		}
		if !reflect.DeepEqual(c.Snapshot(), valid) {
			t.Fatal("failed replacement changed old snapshot", c.Snapshot())
		}
	}
	if c.Get("000001.SZ") == nil {
		t.Fatal("query aliases no longer accepted")
	}
	if err := c.Update(nil); err != nil || len(c.Snapshot()) != 0 {
		t.Fatal("valid empty replacement failed", err)
	}
}
