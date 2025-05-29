package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	ls := tdx.FastHosts(tdx.Hosts...)
	for _, v := range ls {
		logs.Debug(v)
	}
	logs.Debug("总数量:", len(ls))
}
