package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	c, err := tdx.Dial("124.71.187.122:7709")
	logs.PanicErr(err)

	resp, err := c.GetCount(protocol.ExchangeSH)
	logs.PanicErr(err)

	logs.Debug(resp.Count)

	<-c.Done()
}
