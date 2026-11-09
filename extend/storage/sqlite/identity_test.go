package sqlite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatasetIdentityIgnoresPathAliases(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "dataset")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(wd, dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewPull(relative, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := NewPull(dir, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if !filepath.IsAbs(a.Identity()) || a.Identity() != b.Identity() {
		t.Fatal("same dataset has different identities", a.Identity(), b.Identity())
	}
	if err := a.SaveUpdate("shared", 123); err != nil {
		t.Fatal(err)
	}
	if unix, has, err := b.LoadUpdate("shared"); err != nil || !has || unix != 123 {
		t.Fatal("aliases did not open the same dataset", unix, has, err)
	}
	t.Run("symlink", func(t *testing.T) {
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Symlink(dir, alias); err != nil {
			t.Skip("symlink unavailable:", err)
		}
		c, err := New(alias)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if c.Identity() != a.Identity() || c.dir != a.dir {
			t.Fatal("symlink changed dataset identity", c.Identity(), a.Identity())
		}
	})
}
