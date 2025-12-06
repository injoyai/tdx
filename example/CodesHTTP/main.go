package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/extend"
)

func main() {
	go extend.ListenCodesHTTP(10033)

	<-time.After(time.Second * 3)
	c, err := extend.DialCodesHTTP("http://localhost:10033")
	logs.PanicErr(err)

	for _, v := range c.GetStocks() {
		println(v)
	}
}
