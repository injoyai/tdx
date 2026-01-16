package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {

	code := "sh600138"

	c, err := tdx.DialDefault()
	logs.PanicErr(err)

	e, err := tdx.NewGbbq(tdx.WithGbbqClient(c))
	logs.PanicErr(err)

	resp, err := c.GetKlineDay(code, 0, 1)
	logs.PanicErr(err)

	eq := e.GetEquity(code, time.Now())

	logs.Debugf("总股本: %d  流通股本: %d\n", eq.Total, eq.Float)

	logs.Debug("换手率:", e.GetTurnover(code, time.Now(), resp.List[0].Volume*100))
}
