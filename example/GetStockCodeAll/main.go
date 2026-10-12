package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
)

func main() {
	common.Test(func(c tdx.Client) {
		resp, err := c.GetStockCodeAll()
		logs.PanicErr(err)

		for _, v := range resp {
			logs.Debug(v)
		}

		logs.Debug("总数：", len(resp))
	})
}
