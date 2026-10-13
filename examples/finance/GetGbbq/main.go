package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

func main() {
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	resp, err := c.GetGbbq("sz000001")
	logs.PanicErr(err)

	for _, v := range resp.List {
		logs.Debug(v)
	}

	logs.Debug("总数:", len(resp.List))
}
