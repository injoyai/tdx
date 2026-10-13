package main

import (
	"flag"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/examples/common"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	code := flag.String("code", "sz000001", "标准股票或 ETF 代码")
	until := flag.String("until", time.Now().AddDate(0, 0, -7).Format("20060102"), "向前查询到 YYYYMMDD")
	compare := flag.Bool("compare", false, "同时查询普通分钟线，对照 09:30/09:31")
	flag.Parse()
	before, err := time.ParseInLocation("20060102", *until, time.Local)
	logs.PanicErr(err)
	stop := func(k tdx.Kline) bool { return k.Time < before.Format("2006-01-02 15:04") }
	common.Run(func(c tdx.Client) {
		result, err := c.GetKlineMinute241Until(*code, stop)
		logs.PanicErr(err)
		resp, err := result.StandardResponse()
		logs.PanicErr(err)
		for _, minutes := range []int{5, 15, 60} {
			logs.Debug("合并周期:", minutes, "分钟")
			for _, k := range protocol.Klines(resp.List).Merge241(minutes) {
				logs.Debug(k.Time.Format(time.DateTime), k.Volume, k.Amount)
			}
		}
		if *compare {
			ordinary, err := c.GetKlineMinuteUntil(*code, stop)
			logs.PanicErr(err)
			for _, series := range []*tdx.KlineResp{result, ordinary} {
				logs.Debug("分钟记录数:", series.Count)
				for _, k := range series.List {
					switch k.Standard.Time.Format("1504") {
					case "0930", "0931":
						logs.Debug(k.Time, k.Volume, k.Amount)
					}
				}
			}
		}
	})
}
