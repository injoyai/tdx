package sqlite

import (
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
)

func testStore(t *testing.T) *PullStore {
	t.Helper()
	store, err := NewPull(t.TempDir(), 2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	return store
}

var registerMockOnce sync.Once

type mockUnit struct{}

func (*mockUnit) Name() string                                { return "sqlite-test" }
func (*mockUnit) Codes(*pull.Service) ([]model.Code, error)   { return nil, nil }
func (*mockUnit) FetchDay(*pull.Service, model.Code) error    { return nil }
func (*mockUnit) FetchMinute(*pull.Service, model.Code) error { return nil }
