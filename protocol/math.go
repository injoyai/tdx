package protocol

import (
	"math"
)

func float32Value(val uint32) float64 {
	return float64(math.Float32frombits(val))
}

// I64Sqrt 返回整数平方根；x<=0 返回0。
func I64Sqrt(x int64) int64 {
	if x <= 0 {
		return 0
	}
	r := int64(math.Sqrt(float64(x)))
	for r+1 <= x/(r+1) {
		r++
	}
	for r > x/r {
		r--
	}
	return r
}
