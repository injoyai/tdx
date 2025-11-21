package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend"
)

func main() {

	m, err := tdx.NewManage()
	logs.PanicErr(err)

	err = extend.NewPullKline(extend.PullKlineConfig{
		Codes:   []string{"sz000001"},
		Tables:  []string{extend.Year},
		Dir:     filepath.Join(tdx.DefaultDatabaseDir, "kline"),
		Limit:   1,
		StartAt: time.Time{},
	}).Run(context.Background(), m)
	logs.PanicErr(err)

}
