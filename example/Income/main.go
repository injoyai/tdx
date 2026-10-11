package main

import (
	"math"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend"
	"github.com/injoyai/tdx/v2/extend/pull"
	_ "github.com/injoyai/tdx/v2/extend/pull/market"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	code := "sz000001"

	// 先运行 example/PullKline 或 example/PullV2，使用同一目录中的日线数据。
	s, err := pull.NewService(&pull.Config{
		Dir:   "./output/pullv2",
		Codes: []string{code},
		Day:   true,
	})
	logs.PanicErr(err)
	defer s.Close()

	c, err := pull.ParseCode(code)
	logs.PanicErr(err)
	ks, err := s.QueryDay(c, time.Time{}, time.Now())
	logs.PanicErr(err)

	ks2 := make(protocol.Klines, len(ks))
	for i, v := range ks {
		ks2[i] = &protocol.Kline{
			Time:  time.Unix(v.Unix, 0),
			Open:  protocol.Price(math.Round(v.Open * 1000)),
			High:  protocol.Price(math.Round(v.High * 1000)),
			Low:   protocol.Price(math.Round(v.Low * 1000)),
			Close: protocol.Price(math.Round(v.Close * 1000)),
		}
	}

	t := time.Now().AddDate(0, -1, -9)
	logs.Debug(t.Format(time.DateOnly))
	ls := extend.DoIncomes(ks2, t, 5, 10, 20)

	logs.Debug(len(ls))

	for _, v := range ls {
		logs.Info(v)
	}

}
