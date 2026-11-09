package protocol

import (
	"fmt"
)

type PriceLevel struct {
	Buy    bool    //买卖
	Price  float64 // 档位报价，元（指数为点）
	Number int     //量 想买卖的数量
}

type PriceLevels [5]PriceLevel

func (ps PriceLevels) String() string {
	s := ""
	if ps[0].Buy {
		for i, v := range ps {
			s += fmt.Sprintf("买%d  %g元  %s\n", i+1, v.Price, IntUnitString(v.Number))
		}

	} else {
		for i := 4; i >= 0; i-- {
			s += fmt.Sprintf("卖%d  %g元  %s\n", i+1, ps[i].Price, IntUnitString(ps[i].Number))
		}
	}
	return s
}
