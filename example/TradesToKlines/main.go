package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/example/common"
)

func main() {
	common.Test(func(c *tdx.Client) {

		resp, err := c.GetHistoryTradeDay("20251223", "sh600000")
		logs.PanicErr(err)

		ks := resp.List.Klines()

		for _, v := range ks {
			logs.Debug(v.Time.Format(time.TimeOnly), v.Last, v.Open, v.High, v.Low, v.Close, v.Volume, v.Amount)
		}

	})
}
