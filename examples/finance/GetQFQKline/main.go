package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// 同一份日线用于前后复权、因子和换手率计算。
func main() {
	code := flag.String("code", "sh600519", "标准股票代码")
	flag.Parse()
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()
	gb, err := tdx.NewGbbq(tdx.WithGbbqClient(c))
	logs.PanicErr(err)
	defer gb.Close()
	result, err := c.GetKlineDayAll(*code)
	logs.PanicErr(err)
	resp, err := result.StandardResponse()
	logs.PanicErr(err)
	if len(resp.List) == 0 {
		logs.Info("没有可用日线")
		return
	}
	qfq := gb.QFQ(*code, resp.List)
	hfq := gb.HFQ(*code, resp.List)
	for _, k := range qfq[max(0, len(qfq)-5):] {
		fmt.Printf("%s 前复权 开%.2f 高%.2f 低%.2f 收%.2f\n",
			k.Time.Format(time.DateOnly), k.Open.Float64(), k.High.Float64(), k.Low.Float64(), k.Close.Float64())
	}
	for _, k := range hfq[max(0, len(hfq)-5):] {
		fmt.Printf("%s 后复权 收%.2f\n", k.Time.Format(time.DateOnly), k.Close.Float64())
	}
	xrxds := gb.GetXRXDs(*code)
	logs.Debug("除权除息记录:", len(xrxds))
	for _, factor := range gb.GetFactors(*code, resp.List) {
		logs.Debug(factor)
	}
	latest := resp.List[len(resp.List)-1]
	if equity := gb.GetEquity(*code, latest.Time); equity != nil {
		logs.Debug("总股本:", equity.Total, "流通股本:", equity.Float)
		logs.Debug("换手率:", gb.GetTurnover(*code, latest.Time, protocol.ToShares(latest.Volume)))
	} else {
		logs.Warn("该日期没有股本记录，未计算换手率")
	}
}
