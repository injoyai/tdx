package update

import (
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/internal/testutil"
)

func TestTrackerRejectsMissingStore(t *testing.T) {
	for _, store := range []Store{nil, (*testutil.Marks)(nil)} {
		if tracker, err := NewWithStore(store, 15, 1); err == nil || tracker != nil {
			t.Fatal("accepted missing Store", tracker, err)
		}
	}
}

func TestTrackerWithInjectedStore(t *testing.T) {
	store := &testutil.Marks{}
	u, err := NewWithStore(store, 15, 1)
	if err != nil {
		t.Fatal(err)
	}
	if done, err := u.IsUpdated("missing"); err != nil || done || len(store.Marks) != 0 {
		t.Fatal("read created a mark", done, err)
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
	if done, err := u.IsUpdated("first"); err != nil || !done || len(store.Marks) != 1 {
		t.Fatal("concurrent mark lost", done, err)
	}
	cutoff := time.Date(2026, 1, 15, 20, 5, 0, 0, time.UTC)
	for _, tc := range []struct {
		k      string
		offset int64
		want   bool
	}{{"before", -1, false}, {"exact", 0, true}, {"after", 1, true}} {
		if err := store.SaveUpdate(tc.k, cutoff.Unix()+tc.offset); err != nil {
			t.Fatal(err)
		}
		if done, err := u.IsUpdatedSince(tc.k, cutoff); err != nil || done != tc.want {
			t.Fatal(tc.k, done, err)
		}
	}
}
