package pull

import "testing"

func TestToShares(t *testing.T) {
	cases := []struct {
		lots   int64
		shares int64
	}{
		{0, 0},
		{1, 100},
		{10, 1000},
		{12345, 1234500},
	}
	for _, c := range cases {
		if got := ToShares(c.lots); got != c.shares {
			t.Errorf("ToShares(%d) = %d, want %d", c.lots, got, c.shares)
		}
	}
}

func TestFromShares(t *testing.T) {
	cases := []struct {
		shares int64
		lots   int64
	}{
		{0, 0},
		{100, 1},
		{1234500, 12345},
	}
	for _, c := range cases {
		if got := FromShares(c.shares); got != c.lots {
			t.Errorf("FromShares(%d) = %d, want %d", c.shares, got, c.lots)
		}
	}
}

// 往返一致。
func TestShareRoundTrip(t *testing.T) {
	lots := int64(123456)
	if got := FromShares(ToShares(lots)); got != lots {
		t.Errorf("round trip: got %d, want %d", got, lots)
	}
}
