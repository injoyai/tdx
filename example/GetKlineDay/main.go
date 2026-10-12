package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
)

func main() {
	common.Test(func(c *tdx.Client) {
		result, err := c.GetKlineDay("920992", 0, 20)
		logs.PanicErr(err)
		resp, err := result.StandardResponse()
		logs.PanicErr(err)

		for _, v := range resp.List {
			logs.Debug(v)
		}

		logs.Debug("总数：", resp.Count)
	})
}
