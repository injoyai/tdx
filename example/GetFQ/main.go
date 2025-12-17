package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {

	/*
		sz000001 307.86
		sh600887
		sh603156 3.53956
		sh600887 91.3258
	*/
	code := "sh600887"

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

	ks := xs.Pre2(resp.List)

	for _, v := range ks.Factor() {
		logs.Debug(v)
	}

	return

	m := ks.FactorMap()

	for i := range ks {
		ks[i].Kline = ks[i].FQ(m[ks[i].Time.Format(time.DateOnly)].HFQ)
	}

	for _, v := range ks {
		logs.Debug(v.Kline)
	}

}
