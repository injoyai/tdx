package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend"
)

func main() {
	go extend.ListenCodesHTTP(10033)

	<-time.After(time.Second * 3)
	c, err := extend.DialCodesHTTP("http://localhost:10033")
	logs.PanicErr(err)
	defer c.Close()

	for _, v := range c.GetStocks() {
		println(v)
	}
}
