package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	common.Test(func(c tdx.Client) {
		resp, err := c.GetTrade("sz000001", 0, 200)
		logs.PanicErr(err)

		for _, v := range resp.List {
			logs.Debug(v)
		}

		trades := protocol.Trades{}
		for _, v := range resp.List {
			trades = append(trades, v.Standard)
		}
		for _, v := range trades.Klines() {
			logs.Debug(v, v.Order)
		}

		logs.Debug("总数：", resp.Count)
	})
}
