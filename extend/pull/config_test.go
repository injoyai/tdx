package pull

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestServiceRejectsTypedNilDependencies(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(*Config)
	}{
		{"Pool", func(c *Config) { c.Pool = (*tdx.Pool)(nil) }},
		{"ExPool", func(c *Config) { c.ExPool = (*tdx.Pool)(nil) }},
		{"Store", func(c *Config) { c.Store = (*sqlite.PullStore)(nil) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			cfg := &Config{Store: testStore(dir)}
			tc.set(cfg)
			if s, err := New(cfg); err == nil || s != nil || !strings.Contains(err.Error(), tc.name) {
				t.Fatal("accepted typed nil", s, err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("invalid dependency created local files", err)
			}
		})
	}
}

func TestListCodesDoesNotChangeBarScope(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	for _, day := range []bool{false, true} {
		cfg := Config{Store: testStore(t.TempDir()), Codes: []string{"test.one"}, Day: day, Minute: true}
		plain, err := New(&cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer plain.Close()
		cfg.ListCodes = func(ctx context.Context) ([]model.Code, error) { return nil, ctx.Err() }
		enriched, err := New(&cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer enriched.Close()
		if same := plain.updateKey("pull") == enriched.updateKey("pull"); !same {
			t.Fatalf("day=%v: raw bar scope equal=%v", day, same)
		}
	}
}

func TestServiceStartDateValidation(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	for _, value := range []string{"20260230", "20261301", "2026-01-02", "2026012"} {
		t.Run(value, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			s, err := New(&Config{Store: testStore(dir), Codes: []string{"test.one"}, StartAt: value})
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
			s, err := New(&Config{Store: testStore(t.TempDir()), Codes: []string{"test.one"}, StartAt: value})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			want := time.Now().In(protocol.Location()).AddDate(-2, 0, 0).Format("20060102")
			if value != "" {
				want = value
			}
			if s.Start().Format("20060102") != want {
				t.Fatal("incorrect normalized start", s.Start(), want)
			}
		})
	}
}

func TestServiceStartIndependentOfHostTimezone(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	previous := time.Local
	defer func() { time.Local = previous }()
	for _, host := range []*time.Location{time.UTC, time.FixedZone("west", -5*3600)} {
		time.Local = host
		s, err := New(&Config{Store: testStore(t.Name()), Codes: []string{"test.one"}, StartAt: "20261020"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		})
		want := time.Date(2026, 10, 20, 0, 0, 0, 0, protocol.Location())
		if !s.Start().Equal(want) {
			t.Fatal("host changed storage boundary", s.Start(), want)
		}
	}
}
