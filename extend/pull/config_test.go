package pull

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServiceStartDateValidation(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	for _, value := range []string{"20260230", "20261301", "2026-01-02", "2026012"} {
		t.Run(value, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			s, err := NewService(&Config{Dir: dir, Codes: []string{"test.one"}, StartAt: value})
			if err == nil || s != nil {
				t.Fatal("accepted invalid start date", value, err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("invalid configuration created a database directory", err)
			}
		})
	}
	for _, value := range []string{"", "20240229"} {
		t.Run("valid-"+value, func(t *testing.T) {
			s, err := NewService(&Config{Dir: t.TempDir(), Codes: []string{"test.one"}, StartAt: value})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			want := time.Now().AddDate(-2, 0, 0).Format("20060102")
			if value != "" {
				want = value
			}
			if s.Start().Format("20060102") != want {
				t.Fatal("incorrect normalized start", s.Start(), want)
			}
		})
	}
}
