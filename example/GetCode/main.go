package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	common.Test(func(c tdx.Client) {
		resp, err := c.GetCode(protocol.ExchangeSH, 369, 1000)
		logs.PanicErr(err)

		for i, v := range resp.List {
			logs.Debug(i, v, v.Standard.LastPrice)
		}
		logs.Debug("总数:", resp.Count)
	})
}
