package protocol

import (
	"math"
)

func float32Value(val uint32) float64 {
	return float64(math.Float32frombits(val))
}

// I64Sqrt int64版的math.Sqrt
func I64Sqrt(x int64) int64 {
	r := int64(math.Sqrt(float64(x)))
	for (r+1)*(r+1) <= x {
		r++
	}
	for r*r > x {
		r--
	}
	return r
}
