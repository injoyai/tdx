package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {

	for _, v := range tdx.Hosts {

		c, err := tdx.Dial(v, tdx.WithDebug(false))
		if err != nil {
			logs.Errf("[%s] 失败\n", v)
			continue
		}
		c.Close()
		logs.Debugf("[%s] 成功\n", v)

	}

}
