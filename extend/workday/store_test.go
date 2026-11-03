package workday

import (
	"fmt"
	"testing"

	"github.com/injoyai/tdx/v2/lib/xorms"
)

func TestSQLStoreBatchRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			db, err := xorms.NewSQLite(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			store, err := NewSQLStore(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save([]*WorkdayModel{{Unix: 1, Date: "old"}}); err != nil {
				t.Fatal(err)
			}
			rows := make([]*WorkdayModel, 125)
			for i := range rows {
				rows[i] = &WorkdayModel{Unix: int64(i + 2)}
			}
			if fail {
				if _, err := db.Exec("CREATE TRIGGER reject_day BEFORE INSERT ON workday WHEN NEW.Unix=103 BEGIN SELECT RAISE(ABORT, 'reject'); END"); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Save(rows); (err != nil) != fail {
				t.Fatal("unexpected batch result", err)
			}
			stored, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if fail {
				if len(stored) != 1 || stored[0].Date != "old" {
					t.Fatal("failed second batch left partial dates", stored)
				}
			} else if len(stored) != 126 || stored[0].Unix != 1 || stored[125].Unix != 126 {
				t.Fatal("batch lost dates or order", stored)
			}
		})
	}
}
