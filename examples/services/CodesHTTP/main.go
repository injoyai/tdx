package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend/httpserver"
	"github.com/injoyai/tdx/v2/extend/metadatahttp"
)

func main() {
	go httpserver.ListenCodesHTTP(10033)

	<-time.After(time.Second * 3)
	c, err := metadatahttp.DialCodesHTTP("http://localhost:10033")
	logs.PanicErr(err)
	defer c.Close()

	for _, v := range c.GetStocks() {
		println(v)
	}
}
