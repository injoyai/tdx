package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// 扩展行情(TdxExHq, 期货/港股/外盘, 端口 7727)示例。
func main() {
	ex, err := tdx.DialExHqDefault()
	logs.PanicErr(err)
	defer ex.Close()

	markets, err := ex.GetMarkets()
	logs.PanicErr(err)
	for _, m := range markets {
		logs.Infof("市场=%d %s\n", m.Market, m.Name)
	}

	n, err := ex.GetCount(protocol.ExchangeHK)
	logs.PanicErr(err)
	logs.Info("港股品种数量:", n)

	insts, err := ex.GetCode(protocol.ExchangeHK, 0, 10)
	logs.PanicErr(err)
	for _, it := range insts.List {
		logs.Infof("品种 市场=%d 代码=%s 名称=%s\n", it.Instrument.Exchange, it.Instrument.Code, it.Name)
	}
}
