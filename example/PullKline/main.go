package main

import (
	"path/filepath"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend"
)

func main() {
	code := "sz000001"

	m, err := tdx.NewManage(tdx.WithDialGbbqDefault())
	logs.PanicErr(err)

	p := extend.NewPullKline(extend.PullKlineConfig{
		Codes:      []string{code},
		Tables:     extend.AllTable,
		Dir:        filepath.Join(tdx.DefaultDatabaseDir, "kline"),
		Goroutines: 1,
		StartAt:    time.Time{},
	})

	err = p.Update(m)
	logs.PanicErr(err)

	ks, err := p.DayKlines(code)
	logs.PanicErr(err)

	logs.Debug(ks)

}
