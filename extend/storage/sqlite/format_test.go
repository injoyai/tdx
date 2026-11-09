package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageSourceFormatAndLegacyDirectory(t *testing.T) {

	dir := t.TempDir()
	legacy := t.TempDir()
	old := filepath.Join(legacy, "old.db")
	if err := os.WriteFile(old, []byte("unchanged legacy content"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := NewPull(legacy, 2); err == nil || s != nil {
		t.Fatal("accepted unmarked legacy directory", err)
	}
	if bs, err := os.ReadFile(old); err != nil || string(bs) != "unchanged legacy content" {
		t.Fatal("legacy content changed", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, ".time-format")); !os.IsNotExist(err) {
		t.Fatal("marked old directory without migration", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "updated.db")); !os.IsNotExist(err) {
		t.Fatal("modified rejected legacy directory", err)
	}
	// 不认识的标记不能当作东八区继续写。
	if err := os.WriteFile(filepath.Join(dir, ".time-format"), []byte("UTC\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := NewPull(dir, 2); err == nil || s != nil {
		t.Fatal("accepted incompatible timezone", err)
	}
	marked := t.TempDir()
	if err := os.WriteFile(filepath.Join(marked, ".storage-timezone"), []byte("UTC+08:00\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := NewPull(marked, 2); err == nil || s != nil {
		t.Fatal("accepted old Unix format", err)
	}
	if _, err := os.Stat(filepath.Join(marked, ".time-format")); !os.IsNotExist(err) {
		t.Fatal("marked old format as source time", err)
	}
}

func TestOldSourceLayoutIsRejectedWithoutModification(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, ".time-format")
	if err := os.WriteFile(marker, []byte("source-time-v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "updated.db")
	if err := os.WriteFile(old, []byte("old data"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := NewPull(dir, 2); err == nil || s != nil {
		t.Fatal("old case-sensitive naming accepted", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "source-time-v1\n" {
		t.Fatal("old marker changed", err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "old data" {
		t.Fatal("old dataset changed", err)
	}
}
