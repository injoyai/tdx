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
	flag.Parse()
	if *date != "" {
		_, err := time.ParseInLocation("20060102", *date, time.Local)
		logs.PanicErr(err)
	}
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	resp, err := query(c, *code, *date)
	logs.PanicErr(err)
	for _, minute := range resp.List {
		logs.Debug(minute)
	}
	logs.Debug("总数:", resp.Count, "成交量单位:", resp.VolumeUnit)
}

func query(c tdx.Client, code, date string) (*tdx.MinuteResp, error) {
	if date != "" {
		return c.GetHistoryMinute(date, code)
	}
	return c.GetMinute(code)
}
