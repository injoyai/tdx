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
		{
			resp, err := c.GetKlineMinute241Until("sz000001", func(k *protocol.Kline) bool {
				return k.Time.Format("20060102") < "20251222"
			})
			logs.PanicErr(err)
			for _, v := range resp.List {
				switch v.Time.Format("1504") {
				case "0930", "0931":
					logs.Debug(v.Time.Format(time.DateTime), v.Volume, v.Amount)
				}
			}
			logs.Debug("总数:", resp.Count)
		}

		{
			resp, err := c.GetKlineMinuteUntil("sz000001", func(k *protocol.Kline) bool {
				return k.Time.Format("20060102") < "20251222"
			})
			logs.PanicErr(err)
			for _, v := range resp.List {
				switch v.Time.Format("1504") {
				case "0930", "0931":
					logs.Debug(v.Time.Format(time.DateTime), v.Volume, v.Amount)
				}
			}
			logs.Debug("总数:", resp.Count)
		}
	})
}
