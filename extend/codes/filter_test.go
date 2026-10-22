package codes

import (
	"reflect"
	"testing"
)

func TestCodeFilters(t *testing.T) {
	c := NewBase()
	c.Update(CodeModels{
		{Exchange: "sh", Code: "000001"}, {Exchange: "sz", Code: "000001"},
		{Exchange: "sh", Code: "510300"}, {Exchange: "bj", Code: "920001"},
		{Exchange: "sh", Code: "880001"}, {Exchange: "sz", Code: "159001"},
	})
	for _, tc := range []struct {
		name string
		get  func(...int) CodeModels
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
	c.Update(nil)
	if c.GetStocks() != nil || c.GetETFs() != nil || c.GetIndexes() != nil {
		t.Fatal("empty filters changed nil lists")
	}
}
