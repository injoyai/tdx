package main

import (
	"fmt"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	cs, err := tdx.NewCodes2()
	logs.PanicErr(err)

	c := cs.Get("sz000001")

	fmt.Println(c.FloatStock, c.TotalStock)
}
