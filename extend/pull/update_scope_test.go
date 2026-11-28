package pull

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
)

type scopeTestUnit struct {
	mockUnit
	days, minutes []string
}

func (u *scopeTestUnit) FetchDay(_ *Service, c model.Code) error {
	u.days = append(u.days, c.Code)
	return nil
}

func (u *scopeTestUnit) FetchMinute(_ *Service, c model.Code) error {
	u.minutes = append(u.minutes, c.Code)
	return nil
}

func TestUpdateScopesConfiguration(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	for _, must := range []bool{false, true} {
		for _, tt := range []struct {
			name   string
			change func(*Config)
		}{
			{"codes", func(c *Config) { c.Codes = []string{"test.two"} }},
			{"period", func(c *Config) { c.Day, c.Minute = false, true }},
			{"start", func(c *Config) { c.StartAt = "20250101" }},
			{"fallback", func(c *Config) { c.TradeFallback = true }},
		} {
			t.Run(fmt.Sprintf("%s/must=%t", tt.name, must), func(t *testing.T) {
				cfg := Config{Store: testStore(t.TempDir()), Codes: []string{"test.one"}, Day: true, StartAt: "20260101", Goroutines: 1}
				u := &scopeTestUnit{mockUnit: mockUnit{name: "test"}}
				s, err := New(&cfg)
				if err != nil {
					t.Fatal(err)
				}
				s.units = []Unit{u}
				if err := s.update(time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), false); err != nil {
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				tt.change(&cfg)
				s, err = New(&cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				s.units = []Unit{u}
				if err := s.update(time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), must); err != nil {
					t.Fatal(err)
				}
				if cfg.Minute {
					if len(u.days) != 1 || len(u.minutes) != 1 || u.minutes[0] != "one" {
						t.Fatalf("minute task skipped: days=%v, minutes=%v", u.days, u.minutes)
					}
				} else if len(u.days) != 2 || u.days[1] != s.overrides["test"][0].Code {
					t.Fatalf("changed task skipped: days=%v", u.days)
				}
				dayCalls, minCalls := len(u.days), len(u.minutes)
				if err := s.update(time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), must); err != nil {
					t.Fatal(err)
				}
				if len(u.days) != dayCalls || len(u.minutes) != minCalls {
					t.Fatal("same task was not deduplicated")
				}
			})
		}
	}
}

func TestUpdateScopeIgnoresCodeOrder(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	cfg := Config{Store: testStore(t.TempDir()), Codes: []string{"test.one", "test.two"}, StartAt: "20260101"}
	s1, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	cfg.Codes = []string{" test.two ", "test.one"}
	cfg.Day, cfg.Minute = true, true
	s2, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if s1.updateKey("pull") != s2.updateKey("pull") {
		t.Fatal("equivalent task configurations produced different keys")
	}
}

func TestUpdateIgnoresLegacyUnscopedMarkers(t *testing.T) {
	s := testService(t)
	u := &scopeTestUnit{mockUnit: mockUnit{name: "test"}}
	s.units = []Unit{u}
	for _, key := range []string{"pull", "pull:test"} {
		if err := s.store.SaveUpdate(key, time.Now().Unix()); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.update(time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC), false); err != nil {
		t.Fatal(err)
	}
	if len(u.days) != 1 || len(u.minutes) != 1 {
		t.Fatal("legacy markers suppressed scoped task")
	}
}

type identifiedUnit struct {
	scopeTestUnit
	identity string
}

func (u *identifiedUnit) Identity() string { return u.identity }

func TestChangedUnitIdentityRejectsStoredData(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	cfg := Config{Store: testStore(t.TempDir()), Codes: []string{"test.one"}, Day: true, StartAt: "20260101"}
	at := time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC)
	for _, identity := range []string{"market-247-scale-10", "market-247-scale-20"} {
		s, err := New(&cfg)
		if err != nil {
			t.Fatal(err)
		}
		u := &identifiedUnit{scopeTestUnit: scopeTestUnit{mockUnit: mockUnit{name: "test"}}, identity: identity}
		s.units = []Unit{u}
		err = s.update(at, true)
		_ = s.Close()
		if identity == "market-247-scale-10" {
			if err != nil || len(u.days) != 1 {
				t.Fatal(err, u.days)
			}
		} else if err == nil || len(u.days) != 0 {
			t.Fatal("changed definition reused old data", err, u.days)
		}
	}
}

type failingDefinitionStore struct {
	storage.BarStore
	once bool
}

func (s *failingDefinitionStore) SaveUpdate(key string, value int64) error {
	if key == "pull:definition:test" && s.once {
		s.once = false
		return errors.New("injected binding write failure")
	}
	return s.BarStore.SaveUpdate(key, value)
}

func TestDefinitionRetryAfterFailedBinding(t *testing.T) {
	db, err := sqlite.NewPull(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	u := &identifiedUnit{scopeTestUnit: scopeTestUnit{mockUnit: mockUnit{name: "test"}}, identity: "A"}
	wrapped := &Service{store: &failingDefinitionStore{BarStore: db, once: true}}
	if err := wrapped.checkIdentity(u); err == nil {
		t.Fatal("write failure was ignored")
	}
	// Bind B through the wrapped store (SaveUpdate path) after A fails.
	u.identity = "B"
	if err := wrapped.checkIdentity(u); err != nil {
		t.Fatal(err)
	}
	native := &Service{store: db}
	if err := native.checkIdentity(u); err != nil {
		t.Fatal("same definition could not verify through native store", err)
	}
	u.identity = "A"
	if err := wrapped.checkIdentity(u); err == nil {
		t.Fatal("failed definition became valid after a different definition was bound")
	}
	if err := native.checkIdentity(u); err == nil {
		t.Fatal("native store accepted A after B was bound")
	}
}
