package pull

import (
	"fmt"
	"testing"
)

type scopeTestUnit struct {
	mockUnit
	days, minutes []string
}

func (u *scopeTestUnit) FetchDay(_ *Service, c Code) error {
	u.days = append(u.days, c.Code)
	return nil
}

func (u *scopeTestUnit) FetchMinute(_ *Service, c Code) error {
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
				cfg := Config{Dir: t.TempDir(), Codes: []string{"test.one"}, Day: true, StartAt: "20260101", Goroutines: 1}
				u := &scopeTestUnit{mockUnit: mockUnit{name: "test"}}
				s, err := New(&cfg)
				if err != nil {
					t.Fatal(err)
				}
				s.units = []Unit{u}
				if err := s.Update(); err != nil {
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
				if err := s.Update(must); err != nil {
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
				if err := s.Update(must); err != nil {
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
	cfg := Config{Dir: t.TempDir(), Codes: []string{"test.one", "test.two"}, StartAt: "20260101"}
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
		if _, err := s.updated.IsUpdated(key); err != nil {
			t.Fatal(err)
		}
		if err := s.updated.Mark(key); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Update(); err != nil {
		t.Fatal(err)
	}
	if len(u.days) != 1 || len(u.minutes) != 1 {
		t.Fatal("legacy markers suppressed scoped task")
	}
}
