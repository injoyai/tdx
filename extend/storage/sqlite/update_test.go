package sqlite

import (
	"strings"
	"sync"
	"testing"
)

func TestUpdateStoreFirstMarkAndConcurrentWrites(t *testing.T) {
	store, err := NewPull(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.updatedDB
	if unix, has, err := store.LoadUpdate("missing"); err != nil || has || unix != 0 {
		t.Fatal(unix, has, err)
	}
	if n, err := db.Count(new(updateRecord)); err != nil || n != 0 {
		t.Fatal("read created placeholder", n, err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			if err := store.SaveUpdate("first", 123); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if unix, has, err := store.LoadUpdate("first"); err != nil || !has || unix != 123 {
		t.Fatal("first mark lost", unix, has, err)
	}
	if n, err := db.Count(new(updateRecord)); err != nil || n != 1 {
		t.Fatal("duplicate concurrent marks", n, err)
	}
	if err := store.SaveUpdate("first", 456); err != nil {
		t.Fatal(err)
	}
	if unix, has, err := store.LoadUpdate("first"); err != nil || !has || unix != 456 {
		t.Fatal("existing mark not updated", unix, has, err)
	}
}

func TestUpdateStoreReopensExistingMarks(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPull(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUpdate("pull:test", 123); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewPull(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if unix, has, err := store.LoadUpdate("pull:test"); err != nil || !has || unix != 123 {
		t.Fatal("persisted mark lost", unix, has, err)
	}
}

func TestDefinitionClaimLegacyMarkers(t *testing.T) {
	const key = "pull:definition:test"
	a := key + ":" + strings.Repeat("a", 64)
	b := key + ":" + strings.Repeat("b", 64)
	for _, tc := range []struct {
		name  string
		aTime int64
		bTime int64
		bound int64
		match bool
	}{
		{"orphan without binding", 123, 0, 0, true},
		{"unique legacy binding", 0, 456, 456, true},
		{"orphan before binding", 123, 456, 456, false},
		{"ambiguous legacy binding", 456, 456, 456, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewPull(t.TempDir(), 1)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			for k, v := range map[string]int64{key: tc.bound, a: tc.aTime, b: tc.bTime} {
				if v != 0 {
					if err := store.SaveUpdate(k, v); err != nil {
						t.Fatal(err)
					}
				}
			}
			if match, err := store.BindDefinition(key, b, -2); err != nil || match != tc.match {
				t.Fatalf("B matched=%t, err=%v", match, err)
			}
			if match, err := store.BindDefinition(key, a, -1); err != nil || match {
				t.Fatalf("A matched=%t, err=%v", match, err)
			}
		})
	}
}

func TestDefinitionClaimAcrossStores(t *testing.T) {
	dir := t.TempDir()
	first, err := NewPull(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := NewPull(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	const key = "pull:definition:test"
	begin := make(chan struct{})
	var wg sync.WaitGroup
	type result struct {
		token int64
		match bool
		err   error
	}
	results := make(chan result, 2)
	for j, db := range []*PullStore{first, second} {
		wg.Go(func() {
			<-begin
			token := -int64(j + 1)
			match, err := db.BindDefinition(key, "", token)
			results <- result{token, match, err}
		})
	}
	close(begin)
	wg.Wait()
	close(results)
	winner, bound, err := first.LoadUpdate(key)
	if err != nil || !bound || (winner != -1 && winner != -2) {
		t.Fatal(winner, bound, err)
	}
	for r := range results {
		if r.err == nil && r.match != (winner == r.token) {
			t.Fatalf("concurrent claim %+v, winner=%d", r, winner)
		}
		// Competing SQLite connections may return BUSY before the winner commits.
		if match, err := second.BindDefinition(key, "", r.token); err != nil || match != (winner == r.token) {
			t.Fatalf("retry token=%d, match=%t, err=%v, winner=%d", r.token, match, err, winner)
		}
		if err := second.SaveUpdate(key, r.token); (err == nil) != (winner == r.token) {
			t.Fatalf("wrapped retry token=%d, err=%v, winner=%d", r.token, err, winner)
		}
	}
}
