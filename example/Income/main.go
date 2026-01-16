package main

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/extend"
	"github.com/injoyai/tdx/protocol"
)

func main() {
	code := "sz000001"

	pull := extend.NewPullKline(extend.PullKlineConfig{
		Codes:  []string{code},
		Tables: []string{extend.Day},
	})

	//m, err := tdx.NewManage(nil)
	//logs.PanicErr(err)

	//err = pull.Run(context.Background(), m)
	//logs.PanicErr(err)

	ks, err := pull.DayKlines(code)
	logs.PanicErr(err)

	ks2 := make(protocol.Klines, len(ks))
	for i, v := range ks {
		ks2[i] = v.Kline
	}

	t := time.Now().AddDate(0, -1, -9)
	logs.Debug(t.Format(time.DateOnly))
	ls := extend.DoIncomes(ks2, t, 5, 10, 20)

	logs.Debug(len(ls))

	for _, v := range ls {
		logs.Info(v)
	}

}
