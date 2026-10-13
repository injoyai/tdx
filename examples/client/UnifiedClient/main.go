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
		fmt.Printf("%s: %d 根，成交量单位 %s\n", r.Instrument, r.Count, r.VolumeUnit)
		for _, k := range r.List {
			fmt.Printf("%s 开 %.3f 收 %.3f 量 %d\n", k.Time, k.Open, k.Close, k.Volume)
		}
	}
}
