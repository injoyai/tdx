package pull

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestStorageSourceFormatAndLegacyDirectory(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	previous := time.Local
	defer func() { time.Local = previous }()
	dir := t.TempDir()
	cfg := &Config{Dir: dir, Codes: []string{"test.one"}, StartAt: "20261020"}
	for _, host := range []*time.Location{time.UTC, time.FixedZone("west", -5*3600)} {
		time.Local = host
		s, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		want := time.Date(2026, 10, 20, 0, 0, 0, 0, protocol.Location())
		if !s.Start().Equal(want) {
			t.Fatal("host changed storage boundary", s.Start(), want)
		}
		s.Close()
	}
	legacy := t.TempDir()
	old := filepath.Join(legacy, "old.db")
	if err := os.WriteFile(old, []byte("unchanged legacy content"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Dir = legacy
	if s, err := New(cfg); err == nil || s != nil {
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
	cfg.Dir = dir
	if s, err := New(cfg); err == nil || s != nil {
		t.Fatal("accepted incompatible timezone", err)
	}
	marked := t.TempDir()
	if err := os.WriteFile(filepath.Join(marked, ".storage-timezone"), []byte("UTC+08:00\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.Dir = marked
	if s, err := New(cfg); err == nil || s != nil {
		t.Fatal("accepted old Unix format", err)
	}
	if _, err := os.Stat(filepath.Join(marked, ".time-format")); !os.IsNotExist(err) {
		t.Fatal("marked old format as source time", err)
	}
}
