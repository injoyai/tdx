package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	m, err := tdx.NewManage()
	logs.PanicErr(err)

	err = m.Do(func(c *tdx.Client) error {
		resp, err := c.GetIndexDayAll("sh000001")
		if err != nil {
			return err
		}
		for _, v := range resp.List {
			logs.Debug(v)
		}
		return nil
	})
	logs.PanicErr(err)
}
