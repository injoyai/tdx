package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {

	code := "sh688326"

	c, err := tdx.DialDefault()
	logs.PanicErr(err)

	e, err := tdx.NewGbbq(tdx.WithGbbqClient(c))
	logs.PanicErr(err)

	resp, err := c.GetKlineDay(code, 0, 1)
	logs.PanicErr(err)

	t := e.Turnover(code, time.Now(), resp.List[0].Volume*100)

	logs.Debug("换手率:", t)
}
