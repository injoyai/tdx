package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	c, err := tdx.DialDefault()
	if err != nil {
		logs.Err(err)
		return
	}
	defer c.CloseAll()
	resp, err := c.GetCodeAll(protocol.ExchangeBJ)
	if err != nil {
		logs.Err(err)
		return
	}
	for _, v := range resp.List {
		logs.Debug("bj" + v.Instrument.Code)
	}
	logs.Debug("总数量:", len(resp.List))
}
