package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/example/common"
)

func main() {
	common.Test(func(c *tdx.Client) {
		resp, err := c.GetTrade("sz000001", 0, 20)
		logs.PanicErr(err)

		for _, v := range resp.List {
			logs.Debug(v)
		}

		logs.Debug("总数：", resp.Count)
	})
}
