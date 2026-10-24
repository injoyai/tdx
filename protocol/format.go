package protocol

import (
	"fmt"

	"github.com/injoyai/conv"
)

func FloatUnit(f float64) (float64, string) {
	m := []string{"万", "亿"}
	unit := ""
	for i := 0; f > 1e4 && i < len(m); i++ {
		unit = m[i]
		f /= 1e4
	}
	return f, unit
}

func FloatUnitString(f float64) string {
	m := []string{"万", "亿", "万亿", "亿亿", "万亿亿", "亿亿亿"}
	unit := ""
	for i := 0; f > 1e4 && i < len(m); i++ {
		unit = m[i]
		f /= 1e4
	}
	if unit == "" {
		return conv.String(f)
	}
	return fmt.Sprintf("%0.2f%s", f, unit)
}

func IntUnitString(n int) string {
	return FloatUnitString(float64(n))
}

func Int64UnitString(n int64) string {
	return FloatUnitString(float64(n))
}
