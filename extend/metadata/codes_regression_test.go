package metadata

import (
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestCodesSnapshotRace(t *testing.T) {
	cs := NewCodes(nil)
	a := []*model.CodeModel{{Exchange: "sh", Code: "600000"}}
	b := []*model.CodeModel{{Exchange: "sz", Code: "000001"}}
	cs.Update(a)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cs.Update(a)
			cs.Update(b)
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
