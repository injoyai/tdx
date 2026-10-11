package extend

import (
	"github.com/injoyai/tdx/v2/lib/bse"
)

func GetBjCodes() ([]string, error) {
	cs, err := bse.GetCodes()
	if err != nil {
		return nil, err
	}
	ls := []string(nil)
	for _, v := range cs {
		ls = append(ls, "bj"+v.Code)
	}
	return ls, nil
}
