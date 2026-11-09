package metadata

import (
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestSnapshotOwnsRecords(t *testing.T) {
	row := &model.CodeModel{Exchange: "sh", Code: "600000", Name: "original"}
	c, err := NewCodes([]*model.CodeModel{row})
	if err != nil {
		t.Fatal(err)
	}
	row.Name = "changed by producer"
	if c.GetName("sh600000") != "original" {
		t.Fatal("constructor retained input pointers")
	}
	c.Get("sh600000").Name = "changed by reader"
	for _, record := range c.GetStocks() {
		record.Name = "changed filter result"
	}
	for _, record := range c.Iter() {
		record.Name = "changed iteration result"
	}
	if c.GetName("sh600000") != "original" {
		t.Fatal("reader mutated the published snapshot")
	}
}
