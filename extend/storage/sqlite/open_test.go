package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "nested", "data.db")
	db, err := Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filename); err != nil {
		t.Fatal(err)
	}
	tables, err := db.DBMetas()
	if err != nil || len(tables) != 0 {
		t.Fatal("connection created application tables", tables, err)
	}
	if db.GetColumnMapper().Obj2Table("DisplayName") != "DisplayName" || db.DB().Stats().MaxOpenConnections != 1 {
		t.Fatal("unexpected connection defaults")
	}
}

func TestOpenParentError(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(parent, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if db, err := Open(filepath.Join(parent, "data.db")); err == nil || db != nil {
		t.Fatal("ignored parent directory error", err)
	}
}
