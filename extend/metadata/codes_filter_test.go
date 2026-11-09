package metadata

import (
	"reflect"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
)

func TestCodeFilters(t *testing.T) {
	c, err := NewCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Update(model.CodeModels{
		{Exchange: "sh", Code: "000001"}, {Exchange: "sz", Code: "000001"},
		{Exchange: "sh", Code: "510300"}, {Exchange: "bj", Code: "920001"},
		{Exchange: "sh", Code: "880001"}, {Exchange: "sz", Code: "159001"},
	}); err != nil {
		t.Error(err)
	}
	for _, tc := range []struct {
		name string
		get  func(...int) model.CodeModels
		want []string
	}{
		{"stocks", c.GetStocks, []string{"sz000001", "bj920001"}},
		{"etfs", c.GetETFs, []string{"sh510300", "sz159001"}},
		{"indexes", c.GetIndexes, []string{"sh000001", "sh880001"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, limits := range [][]int{nil, {0}, {-1}, {10}, {1}, {2}, {1, 2}} {
				want := tc.want
				if len(limits) > 0 && limits[0] == 1 {
					want = want[:1]
				}
				got := tc.get(limits...).Codes()
				if !reflect.DeepEqual(got, want) {
					t.Fatal(limits, got, want)
				}
				t.Logf("fixture %v: %v", limits, got)
			}
		})
	}
	if err := c.Update(nil); err != nil {
		t.Error(err)
	}
	if c.GetStocks() != nil || c.GetETFs() != nil || c.GetIndexes() != nil {
		t.Fatal("empty filters changed nil lists")
	}
}
