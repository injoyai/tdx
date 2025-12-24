package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/example/common"
	"github.com/injoyai/tdx/protocol"
)

func main() {
	common.Test(func(c *tdx.Client) {

		resp, err := c.GetHistoryTradeDay("20251223", "sh600000")
		logs.PanicErr(err)

		ks := resp.List.Klines()

		p := func(v *protocol.Kline) {
			logs.Debug(v.Time.Format(time.TimeOnly), v.Last, v.Open, v.High, v.Low, v.Close, v.Volume, v.Amount)
		}

		for _, v := range ks {
			p(v)
		}

		for _, v := range ks.Merge241(5) {
			p(v)
		}

		for _, v := range ks.Merge241(15) {
			p(v)
		}

		for _, v := range ks.Merge241(60) {
			p(v)
		}

		ks = protocol.Klines{ks[0], ks[1], ks[2]}
		for _, v := range ks.Merge241(60) {
			p(v)
		}

	})
}
