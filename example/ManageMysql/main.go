package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	_, err := tdx.NewManageMysql(
		"root:root@tcp(192.168.1.105:3306)/stock?charset=utf8mb4&parseTime=True&loc=Local",
		tdx.WithClients(2),
	)
	logs.PanicErr(err)
	logs.Debug("done")
}
