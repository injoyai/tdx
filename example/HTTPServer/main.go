package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend/httpserver"
)

func main() {
	s, err := httpserver.Default()
	logs.PanicErr(err)
	s.Run()
}
