package extend

import (
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2"
)

func TestCodesHTTPStartupErrors(t *testing.T) {
	want := errors.New("offline initialization failure")
	options := []tdx.CodesOption{tdx.WithCodesDialClient(func() (*tdx.Client, error) { return nil, want })}
	for _, test := range []struct {
		name string
		fn   func() error
	}{
		{"codes", func() error { return ListenCodesHTTP(0, options...) }},
		{"codes and equity", func() error { return ListenCodesAndGbbqHTTP(0, options, nil) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); !errors.Is(err, want) {
				t.Errorf("initialization failure reported as success: got=%v", err)
			}
		})
	}
}
