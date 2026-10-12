package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/example/common"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	common.Test(func(c tdx.Client) {
		old := time.Now().Add(-time.Hour * 24 * 17)
		result, err := c.GetKlineUntil(protocol.TypeKlineDay, "sz000001", func(record tdx.MarketKline) bool {
			k := record.Standard
			return k.Time.Sub(old) < 0
		})
		logs.PanicErr(err)
		resp, err := result.StandardResponse()
		logs.PanicErr(err)
		for _, v := range resp.List {
			logs.Debug(v)
		}
		logs.Debug(resp.Count)
	})
}
