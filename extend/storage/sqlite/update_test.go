package sqlite

import (
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
