package main

import (
	"flag"
	"os"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/lib/gbbq"
)

func main() {
	file := flag.String("file", "./output/gbbq/gbbq/gbbq", "已解压的股本文件")
	code := flag.String("code", "000001", "不带市场前缀的股票代码")
	flag.Parse()
	bs, err := os.ReadFile(*file)
	logs.PanicErr(err)
	records, err := gbbq.Decode(bs)
	logs.PanicErr(err)
	stocks := 0
	for _, record := range records {
		if record.Code == *code {
			logs.Debug(record.Date.Format(time.DateOnly), record.Category, record.C1, record.C2, record.C3, record.C4)
			stocks++
		}
	}
	logs.Debug("总数:", stocks)
}
