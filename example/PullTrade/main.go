package main

import (
	"context"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend"
)

func main() {

	pt := extend.NewPullTrade("./output/trade")

	m, err := tdx.NewManage()
	logs.PanicErr(err)
	if closer, ok := m.IPool.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	err = pt.PullYear(context.Background(), m, 2025, "sz000001")
	logs.Err(err)

}
