package main

import (
	"context"
	"flag"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend"
	"github.com/injoyai/tdx/v2/extend/manage"
)

func main() {
	code := flag.String("code", "sz000001", "标准股票或 ETF 代码")
	year := flag.Int("year", time.Now().Year(), "历史分笔年份")
	flag.Parse()
	if *year < 1990 || *year > time.Now().Year() {
		logs.Panic("年份超出范围")
	}

	pt := extend.NewPullTrade("./output/trade")

	m, err := manage.New()
	logs.PanicErr(err)
	defer m.Close()

	err = pt.PullYear(context.Background(), m, *year, *code)
	logs.Err(err)

}
