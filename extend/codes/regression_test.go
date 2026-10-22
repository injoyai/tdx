package codes

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type snapshotClient struct {
	tdx.Client
	remote []*protocol.Code
}

func (c *snapshotClient) GetCodeAll(ex protocol.Exchange) (*tdx.CodeResp, error) {
	r := &tdx.CodeResp{}
	if ex == protocol.ExchangeSH {
		for _, v := range c.remote {
			r.List = append(r.List, tdx.Code{Standard: v})
		}
	}
	r.Count = len(r.List)
	return r, nil
}
func TestMetadataSnapshot(t *testing.T) {
	for _, mode := range []string{"refresh", "unchanged", "zero metadata", "bulk", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			db, err := xorms.NewSQLite(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = db.Sync2(new(CodeModel)); err != nil {
				t.Fatal(err)
			}
			old := []*CodeModel{{Exchange: "sh", Code: "600000", Name: "old", Decimal: 2, Multiple: 100, LastPrice: 10}, {Exchange: "sh", Code: "600001", Name: "delisted", Decimal: 2}}
			if _, err = db.Insert(old); err != nil {
				t.Fatal(err)
			}
			old = nil
			if err := db.Find(&old); err != nil {
				t.Fatal(err)
			}
			updated, err := update.New(db, 9, 0)
			if err != nil {
				t.Fatal(err)
			}
			remote := []*protocol.Code{{Code: "600000", Name: "new", Decimal: 2}}
			switch mode {
			case "unchanged":
				remote = []*protocol.Code{{Code: "600000", Name: "old", Decimal: 2, Multiple: 100, LastPrice: 10}, {Code: "600001", Name: "delisted", Decimal: 2}}
			case "zero metadata":
				remote = []*protocol.Code{{Code: "600000", Name: "old"}}
			case "bulk":
				for i := 100; i < 205; i++ {
					remote = append(remote, &protocol.Code{Code: fmt.Sprintf("600%03d", i), Name: "added", Decimal: 2})
				}
			case "rollback":
				remote = append(remote, &protocol.Code{Code: "600002", Name: "reject"})
				if _, err := db.Exec("CREATE TRIGGER reject_new BEFORE INSERT ON codes WHEN NEW.code='600002' BEGIN SELECT RAISE(ABORT, 'reject'); END"); err != nil {
					t.Fatal(err)
				}
			}
			cs := &Codes{c: &snapshotClient{remote: remote}, db: db, updated: updated, updateKey: "codes", Base: NewBase()}

			cs.Base.Update(old)
			err = cs.Update()
			if mode == "rollback" {
				if err == nil {
					t.Fatal("insert failure did not roll back")
				}
				var stored []*CodeModel
				if err := db.Find(&stored); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(stored, old) || cs.GetName("sh600000") != "old" || cs.Get("sh600001") == nil {
					t.Fatal("failure changed database or cache", stored)
				}
				if done, err := updated.IsUpdated("codes"); done || err != nil {
					t.Fatal("failure marked refresh complete", done, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var stored []*CodeModel
			if err := db.Find(&stored); err != nil {
				t.Fatal(err)
			}
			if len(stored) != len(remote) || !reflect.DeepEqual(stored, []*CodeModel(cs.GetStocks())) {
				t.Fatal("cache differs from committed database", stored, cs.GetStocks())
			}
			if cs.Get("sh600000").ID != old[0].ID || cs.Get("sh600000").InDate != old[0].InDate {
				t.Fatal("refresh replaced existing identity")
			}
			if mode == "unchanged" && !reflect.DeepEqual(stored, old) {
				t.Fatal("unchanged refresh lost records", stored)
			}
			if mode == "zero metadata" {
				row := cs.Get("sh600000")
				if row.Decimal != 0 || row.Multiple != 0 || row.LastPrice != 0 {
					t.Fatal("zero metadata was not updated", row)
				}
			}
			if mode == "refresh" && (cs.Get("sh600001") != nil || cs.GetName("sh600000") != "new") {
				t.Fatal("delisted or renamed code remained stale")
			}
		})
	}
}

func TestCodesSnapshotRace(t *testing.T) {
	cs := NewBase()
	a := []*CodeModel{{Exchange: "sh", Code: "600000"}}
	b := []*CodeModel{{Exchange: "sz", Code: "000001"}}
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
