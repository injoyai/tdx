package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

// 扩展行情(TdxExHq, 期货/港股/外盘, 端口 7727)示例。
func main() {
	ex, err := tdx.DialExHqDefault()
	logs.PanicErr(err)
	defer ex.Close()

	markets, err := ex.ExMarkets()
	logs.PanicErr(err)
	for _, m := range markets {
		logs.Infof("市场=%d %s\n", m.Market, m.Name)
	}

	n, err := ex.ExCount()
	logs.PanicErr(err)
	logs.Info("品种数量:", n)

	insts, err := ex.ExInstruments(0, 10)
	logs.PanicErr(err)
	for _, it := range insts {
		logs.Infof("品种 市场=%d 代码=%s 名称=%s\n", it.Market, it.Code, it.Name)
	}
}
