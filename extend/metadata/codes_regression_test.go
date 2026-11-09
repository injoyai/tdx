package metadata

import (
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestCodesSnapshotRace(t *testing.T) {
	cs, err := NewCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	a := []*model.CodeModel{{Exchange: "sh", Code: "600000"}}
	b := []*model.CodeModel{{Exchange: "sz", Code: "000001"}}
	if err := cs.Update(a); err != nil {
		t.Error(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			if err := cs.Update(a); err != nil {
				t.Error(err)
			}
			if err := cs.Update(b); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = cs.GetStocks()
			_ = cs.GetETFs()
			_ = cs.GetIndexes()
			for code := range cs.Iter() {
				_ = cs.Get(code)
			}
		}
	}()
	wg.Wait()
}
