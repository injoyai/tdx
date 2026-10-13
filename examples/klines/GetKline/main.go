package main

import (
	"flag"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	code := flag.String("code", "sz000001", "证券代码；指数如 sh000001")
	category := flag.Uint("type", uint(protocol.TypeKlineDay), "周期：0=5分，1=15分，2=30分，3=60分，5=周，6=月，7=1分，9=日，10=季，11=年")
	start := flag.Uint("start", 0, "从最新端的偏移，0..65535")
	count := flag.Uint("count", 20, "单页数量，1..800")
	all := flag.Bool("all", false, "查询全部可用历史")
	until := flag.String("until", "", "向前查询到 YYYYMMDD，包含首次命中停止条件的记录")
	flag.Parse()
	if *category > 11 || *start > 65535 || *count == 0 || *count > 800 {
		logs.Panic("周期或分页参数超出范围")
	}
	if *all && *until != "" {
		logs.Panic("-all 与 -until 不能同时使用")
	}
	var before time.Time
	if *until != "" {
		var err error
		before, err = time.ParseInLocation("20060102", *until, time.Local)
		logs.PanicErr(err)
	}
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	resp, err := query(c, uint8(*category), *code, uint16(*start), uint16(*count), *all, before)
	logs.PanicErr(err)
	logs.Infof("%s: %d 根，成交量单位 %s", resp.Instrument, resp.Count, resp.VolumeUnit)
	for _, k := range resp.List {
		logs.Debug(k)
	}
}

func query(c tdx.Client, category uint8, code string, start, count uint16, all bool, before time.Time) (*tdx.KlineResp, error) {
	if !before.IsZero() {
		boundary := before.Format("2006-01-02 15:04")
		return c.GetKlineUntil(category, code, func(k tdx.Kline) bool { return k.Time < boundary })
	}
	if all {
		return c.GetKlineAll(category, code)
	}
	return c.GetKline(category, code, start, count)
}
