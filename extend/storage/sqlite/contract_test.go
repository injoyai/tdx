package sqlite

import (
	"testing"

	"github.com/injoyai/tdx/v2/extend/storage/storagetest"
)

func TestBarStoreContract(t *testing.T) {
	storagetest.Run(t, func(t *testing.T) storagetest.Fixture {
		s := testStore(t)
		return storagetest.Fixture{Store: s, Fail: func(t *testing.T, failure storagetest.Failure) {
			filename, table := s.MinuteFile(failure.Code, failure.Year), "KlineMinute"
			if failure.Point == storagetest.DayInsert {
				filename, table = s.DayFile(failure.Code), "KlineDay"
			} else if failure.Point == storagetest.CoverageWrite {
				table = "minuteCoverage"
			}
			db, err := Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			if _, err := db.Exec("CREATE TRIGGER reject_contract BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'contract failure'); END"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := db.Exec("DROP TRIGGER reject_contract"); err != nil {
					t.Error(err)
				}
			})
		}}
	})
}
