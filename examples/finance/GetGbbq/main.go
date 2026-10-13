package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/examples/common"
)

func main() {
	common.Run(func(c tdx.Client) {
		resp, err := c.GetGbbq("sz000001")
		logs.PanicErr(err)

		for _, v := range resp.List {
			logs.Debug(v)
		}

		logs.Debug("总数:", len(resp.List))

	})
}
