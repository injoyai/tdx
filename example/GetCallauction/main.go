package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()
	resp, err := c.GetCallAuction("sz000001")
	logs.PanicErr(err)

	for _, v := range resp.List {
		logs.Debug(v)
	}
	logs.Debug(resp.Count, len(resp.List))
}
