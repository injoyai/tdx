package extend

import (
	"github.com/injoyai/tdx"
)

func GetBjCodes() ([]*tdx.BjCode, error) {
	return tdx.GetBjCodes()
}
