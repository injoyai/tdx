package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

// 同一份日线用于前后复权、因子和换手率计算。
func main() {
	code := flag.String("code", "sh600519", "标准股票代码")
	flag.Parse()
	pool, err := tdx.NewPool(func() (tdx.Client, error) { return tdx.DialDefault(tdx.WithRedial(false)) }, 1)
	logs.PanicErr(err)
	defer pool.Close()
	store, err := sqlite.New("./output/adjustments")
	logs.PanicErr(err)
	defer store.Close()
	syncer, err := pull.New(&pull.Config{Pool: pool, GbbqStore: store})
	logs.PanicErr(err)
	defer syncer.Close()
	snapshot, err := syncer.SyncGbbq(context.Background(), []string{*code})
	logs.PanicErr(err)
	gb, err := metadata.NewGbbq(snapshot.Rows)
	logs.PanicErr(err)
	var result *tdx.KlineResp
	err = pool.Do(func(c tdx.Client) error { var err error; result, err = c.GetKlineDayAll(*code); return err })
	logs.PanicErr(err)
	ks, err := result.Klines()
	logs.PanicErr(err)
	if len(ks) == 0 {
		logs.Info("没有可用日线")
		return
	}
	qfq := gb.QFQ(*code, ks)
	hfq := gb.HFQ(*code, ks)
	for _, k := range qfq[max(0, len(qfq)-5):] {
		fmt.Printf("%s 前复权 开%.2f 高%.2f 低%.2f 收%.2f\n",
			k.Time.Format(time.DateOnly), k.Open, k.High, k.Low, k.Close)
	}
	for _, k := range hfq[max(0, len(hfq)-5):] {
		fmt.Printf("%s 后复权 收%.2f\n", k.Time.Format(time.DateOnly), k.Close)
	}
	xrxds := gb.GetXRXDs(*code)
	logs.Debug("除权除息记录:", len(xrxds))
	for _, factor := range gb.GetFactors(*code, ks) {
		logs.Debug(factor)
	}
	latest := ks[len(ks)-1]
	if equity := gb.GetEquity(*code, latest.Time); equity != nil {
		logs.Debug("总股本:", equity.Total, "流通股本:", equity.Float)
		if rate, ok := gb.Turnover(*code, latest.Time, protocol.ToShares(latest.Volume)); ok {
			logs.Debug("换手率:", rate)
		}
	} else {
		logs.Warn("该日期没有股本记录，未计算换手率")
	}
}
