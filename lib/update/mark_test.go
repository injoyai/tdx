package update

import (
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/lib/xorms"
)

func TestTrackerFirstMarkAndConcurrentWrites(t *testing.T) {
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, err := New(db, 15, 1)
	if err != nil {
		t.Fatal(err)
	}
	if done, err := u.IsUpdated("missing"); err != nil || done {
		t.Fatal(done, err)
	}
	if n, err := db.Count(new(Record)); err != nil || n != 0 {
		t.Fatal("read created placeholder", n, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			if err := u.Mark("first"); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if done, err := u.IsUpdated("first"); err != nil || !done {
		t.Fatal("first mark lost", done, err)
	}
	if n, err := db.Count(new(Record)); err != nil || n != 1 {
		t.Fatal("duplicate concurrent marks", n, err)
	}
}
