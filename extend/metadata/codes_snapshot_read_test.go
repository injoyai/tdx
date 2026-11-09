package metadata

import (
	"strconv"
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestCodesSnapshotIsCompleteAndIndependent(t *testing.T) {
	rows := model.CodeModels{{Exchange: "sh", Code: "113001", Name: "bond"}, {Exchange: "sh", Code: "019001", Name: "other"}}
	c, err := NewCodes(rows)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Snapshot()
	if len(got) != 2 || got[0].FullCode() != "sh113001" || got[1].FullCode() != "sh019001" {
		t.Fatal("snapshot dropped non-stock securities", got)
	}
	got[0].Name, rows[1].Name = "changed", "changed"
	if c.GetName("sh113001") != "bond" || c.GetName("sh019001") != "other" {
		t.Fatal("snapshot shares mutable records")
	}
}

func TestCodesSnapshotDoesNotMixUpdates(t *testing.T) {
	c, err := NewCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			name := strconv.Itoa(i)
			if err := c.Update(model.CodeModels{{Exchange: "sh", Code: "600000", Name: name}, {Exchange: "sh", Code: "113001", Name: name}}); err != nil {
				t.Error(err)
			}
		}
	}()
	defer wg.Wait()
	for i := 0; i < 200; i++ {
		rows := c.Snapshot()
		if len(rows) != 0 && (len(rows) != 2 || rows[0].Name != rows[1].Name) {
			t.Fatal("mixed snapshot versions", rows)
		}
	}
}
