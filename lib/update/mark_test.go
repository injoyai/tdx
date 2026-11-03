package update

import (
	"sync"
	"testing"
	"time"

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

func TestIsUpdatedSince(t *testing.T) {
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u, err := New(db, 15, 1)
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Date(2026, 1, 15, 20, 5, 0, 0, time.UTC)
	for _, tc := range []struct {
		key    string
		offset int64
		want   bool
	}{
		{"before", -1, false}, {"exact", 0, true}, {"after", 1, true},
	} {
		if _, err := db.Insert(&Record{Key: tc.key, Time: cutoff.Unix() + tc.offset}); err != nil {
			t.Fatal(err)
		}
		if done, err := u.IsUpdatedSince(tc.key, cutoff); err != nil || done != tc.want {
			t.Fatalf("%s: done=%v err=%v", tc.key, done, err)
		}
	}
	if done, err := u.IsUpdatedSince("missing", cutoff); err != nil || done {
		t.Fatal(done, err)
	}
	if count, err := db.Count(new(Record)); err != nil || count != 3 {
		t.Fatal("query created placeholder", count, err)
	}
}
