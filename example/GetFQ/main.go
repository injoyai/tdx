package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {

	/*
		sz000001 145.9241463590320800
		sh603156 3.8565034624713000
		sh600887 105.5060784942809000
	*/
	code := "sz000001"

	c, err := tdx.DialDefault()
	logs.PanicErr(err)

	gb, err := tdx.NewGbbq(tdx.WithGbbqClient(c))
	logs.PanicErr(err)

	xs := gb.GetXRXDs(code)

	for _, v := range xs {
		logs.Info(v)
	}
	logs.Info("总数:", len(xs))

	resp, err := c.GetKlineDayAll(code)
	logs.PanicErr(err)

	ks := xs.Pre(resp.List)

	for _, v := range ks.Factors() {
		if v.Last == v.PreLast {
			//continue
		}
		logs.Debug(v)
	}
}
