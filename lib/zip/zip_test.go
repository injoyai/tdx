package zip

import (
	archive "archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func writeArchive(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	var b bytes.Buffer
	z := archive.NewWriter(&b)
	h := &archive.FileHeader{Name: name}
	h.SetMode(mode)
	w, err := z.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("new contents")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "input.zip")
	if err := os.WriteFile(file, b.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	return file
}

func TestDecodeConfinesFiles(t *testing.T) {
	for _, name := range []string{"../escaped.txt", "/escaped.txt", "a/../../escaped.txt", `..\escaped.txt`} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := writeArchive(t, dir, name, 0600)
			if err := Decode(file, filepath.Join(dir, "out")); err == nil {
				t.Fatal("unsafe archive path accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "escaped.txt")); !os.IsNotExist(err) {
				t.Fatal("outside file created", err)
			}
		})
	}
	for _, name := range []string{"nested/file.txt", "./nested/file.txt", "nested/sub/../file.txt"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			file := writeArchive(t, dir, name, 0600)
			if err := Decode(file, out); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(out, "nested/file.txt")); err != nil || string(b) != "new contents" {
				t.Fatal(string(b), err)
			}
		})
	}

}

func TestDecodeRejectsSymlinkEscapes(t *testing.T) {
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "out")
			if err := os.Mkdir(out, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(dir, "sentinel")
			if err := os.WriteFile(sentinel, []byte("preserve"), 0600); err != nil {
				t.Fatal(err)
			}
			name, target := "link", sentinel
			if directory {
				name, target = "link/sentinel", dir
			}
			if err := os.Symlink(target, filepath.Join(out, "link")); err != nil {
				t.Skip("symlink unavailable:", err)
			}
			if err := Decode(writeArchive(t, dir, name, 0600), out); err == nil {
				t.Fatal("existing symlink escaped root")
			}
			if b, err := os.ReadFile(sentinel); err != nil || string(b) != "preserve" {
				t.Fatal("outside file changed", err)
			}
		})
	}
	dir := t.TempDir()
	if err := Decode(writeArchive(t, dir, "link", os.ModeSymlink|0600), filepath.Join(dir, "out")); err == nil {
		t.Fatal("archive symlink accepted")
	}
}

func TestEncodeProtectsSource(t *testing.T) {
	for _, kind := range []string{"same", "relative", "hardlink", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.txt")
			const contents = "preserve source contents"
			if err := os.WriteFile(source, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			target := source
			switch kind {
			case "relative":
				t.Chdir(dir)
				target = "./source.txt"
			case "hardlink", "symlink":
				target = filepath.Join(dir, "alias.zip")
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(source, target); err != nil {
					t.Skip(err)
				}
			}
			if err := Encode(source, target); err == nil {
				t.Fatal("same source accepted")
			}
			if got, err := os.ReadFile(source); err != nil || string(got) != contents {
				t.Fatal("source changed", string(got), err)
			}
		})
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	want := bytes.Repeat([]byte("fixed sample\n"), 100)
	if err := os.WriteFile(source, want, 0600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "nested", "archive.zip")
	if err := Encode(source, target); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	if err := Decode(target, out); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(out, "source.txt")); err != nil || !bytes.Equal(got, want) {
		t.Fatal("round trip mismatch", err)
	}
	if got, err := os.ReadFile(source); err != nil || !bytes.Equal(got, want) {
		t.Fatal("source changed", err)
	}
}
