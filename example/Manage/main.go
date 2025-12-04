package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	m, err := tdx.NewManage()
	logs.PanicErr(err)

	logs.Debug(m.Equity.Get("sz000001", time.Now()))

	err = m.Do(func(c *tdx.Client) error {
		resp, err := c.GetIndexDay("sh000001", 0, 20)
		if err != nil {
			return err
		}
		for _, v := range resp.List {
			_ = v
			//logs.Debug(v)
		}
		return nil
	})
	logs.PanicErr(err)
}
