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
	code := flag.String("code", "sh600000", "标准股票或 ETF 代码")
	date := flag.String("date", "", "必填：历史交易日 YYYYMMDD")
	flag.Parse()
	_, err := time.ParseInLocation("20060102", *date, time.Local)
	logs.PanicErr(err)
	common.Run(func(c tdx.Client) {
		result, err := c.GetHistoryTradeDay(*date, *code)
		logs.PanicErr(err)
		resp, err := result.StandardResponse()
		logs.PanicErr(err)
		ks := resp.List.Klines()
		print := func(k *protocol.Kline) {
			logs.Debug(k.Time.Format(time.TimeOnly), k.Open, k.High, k.Low, k.Close, k.Volume, k.Amount)
		}
		logs.Debug("合成 1 分钟线:", len(ks))
		for _, k := range ks {
			print(k)
		}
		for _, minutes := range []int{5, 15, 60} {
			logs.Debug("合并周期:", minutes, "分钟")
			for _, k := range ks.Merge241(minutes) {
				print(k)
			}
		}
	})
}
