package main

import (
	"path/filepath"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend"
)

func main() {
	code := "sz000001"

	m, err := tdx.NewManage(tdx.WithDialGbbqDefault())
	logs.PanicErr(err)

	p, err := extend.NewPullKline(extend.PullKlineConfig{
		Codes:      []string{code},
		Types:      []string{extend.Day, extend.Minute},
		Dir:        filepath.Join(tdx.DefaultDatabaseDir),
		Goroutines: 1,
		StartAt:    time.Time{},
	})
	logs.PanicErr(err)

	err = p.Update(m)
	logs.PanicErr(err)

	ks, err := p.DayKlines(code, time.Time{}, time.Now())
	logs.PanicErr(err)

	for _, v := range ks {
		_ = v
		logs.Debug(v)
	}

}
