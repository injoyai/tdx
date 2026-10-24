package main

import (
	"fmt"
	"github.com/injoyai/tdx/v2"
)

func main() {
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()
	for _, code := range []string{"sz000001", "sh000001", "hk00700", "usAAPL"} {
		r, err := c.GetKlineDay(code, 0, 5)
		if err != nil {
			panic(err)
		}
		fmt.Printf("%s: %d 根，成交量使用最小单位（股票为股，期货为张）\n", r.Instrument, r.Count)
		for _, k := range r.List {
			fmt.Printf("%s 开 %.3f 收 %.3f 量 %d\n", k.Time, k.Open, k.Close, k.Volume)
		}
	}
}
