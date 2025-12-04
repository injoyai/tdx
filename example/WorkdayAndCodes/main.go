package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/example/common"
)

func main() {
	common.Test(func(c *tdx.Client) {

		_, err := tdx.NewWorkday(tdx.WithWorkdayClient(c))
		logs.PanicErr(err)

		_, err = tdx.NewCodesSqlite(tdx.WithCodesClient(c))
		logs.PanicErr(err)

		c.Close()
	})
}
