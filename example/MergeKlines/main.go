package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	common.Test(func(c *tdx.Client) {
		resp, err := c.GetKlineMinute241Until("sz000001", func(k *protocol.Kline) bool {
			return k.Time.Format("20060102") < "20260223"
		})
		logs.PanicErr(err)

		ks := protocol.Klines(resp.List)

		for _, v := range ks.Merge241(5) {
			logs.Debug(v.Time.Format(time.DateTime), v.Volume, v.Amount)
		}

	})
}
