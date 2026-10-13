package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

func main() {
	ls := tdx.SortHosts(tdx.Hosts)
	logs.Debug("总数量:", len(ls))
}
