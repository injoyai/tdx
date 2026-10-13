package main

import (
	"flag"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

func main() {
	code := flag.String("code", "sz000001", "证券代码")
	date := flag.String("date", "", "历史交易日 YYYYMMDD；留空查询当日")
	start := flag.Uint("start", 0, "从最新端的偏移，0..65535")
	count := flag.Uint("count", 200, "单页数量，实时 1..1800，历史 1..2000")
	all := flag.Bool("all", false, "查询整日分笔")
	flag.Parse()
	limit := uint(1800)
	if *date != "" {
		_, err := time.ParseInLocation("20060102", *date, time.Local)
		logs.PanicErr(err)
		limit = 2000
	}
	if *start > 65535 || *count == 0 || *count > limit {
		logs.Panic("分页参数超出范围")
	}
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	resp, err := query(c, *code, *date, uint16(*start), uint16(*count), *all)
	logs.PanicErr(err)
	for _, trade := range resp.List {
		logs.Debug(trade)
	}
	logs.Debug("总数:", resp.Count, "成交量单位:", resp.VolumeUnit)
}

func query(c tdx.Client, code, date string, start, count uint16, all bool) (*tdx.TradeResp, error) {
	if date != "" {
		if all {
			return c.GetHistoryTradeDay(date, code)
		}
		return c.GetHistoryTrade(date, code, start, count)
	}
	if all {
		return c.GetTradeAll(code)
	}
	return c.GetTrade(code, start, count)
}
