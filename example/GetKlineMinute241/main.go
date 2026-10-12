package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
)

func main() {
	common.Test(func(c *tdx.Client) {
		{
			result, err := c.GetKlineMinute241Until("sz000001", func(record tdx.MarketKline) bool {
				k := record.Standard
				return k.Time.Format("20060102") < "20251222"
			})
			logs.PanicErr(err)
			resp, err := result.StandardResponse()
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
			result, err := c.GetKlineMinuteUntil("sz000001", func(record tdx.MarketKline) bool {
				k := record.Standard
				return k.Time.Format("20060102") < "20251222"
			})
			logs.PanicErr(err)
			resp, err := result.StandardResponse()
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
