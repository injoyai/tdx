package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/pull"
	_ "github.com/injoyai/tdx/v2/extend/pull/market"
)

func main() {
	code := "sz000001"

	m, err := tdx.NewManage(tdx.WithDialGbbqDefault())
	logs.PanicErr(err)
	if closer, ok := m.IPool.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	s, err := pull.NewService(&pull.Config{
		Codes:      []string{code},
		Day:        true,
		Minute:     true,
		Dir:        "./output/pullv2",
		Goroutines: 1,
		Manage:     m,
		Workday:    m.Workday,
	})
	logs.PanicErr(err)
	defer s.Close()

	err = s.Update()
	logs.PanicErr(err)

	c, err := pull.ParseCode(code)
	logs.PanicErr(err)
	ks, err := s.QueryDay(c, time.Time{}, time.Now())
	logs.PanicErr(err)

	for _, v := range ks {
		logs.Debug(v)
	}

}
