package main

import (
	"fmt"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

// 演示一站式获取复权日线(对齐通达信桌面端, 四舍五入到分)。
func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	gb, err := tdx.NewGbbq(tdx.WithGbbqClient(c))
	logs.PanicErr(err)

	code := "sh600519"

	// 方式一: 一步到位拉取全量历史 + 前复权
	qfq, err := gb.QFQKlineDay(code)
	logs.PanicErr(err)
	fmt.Printf("%s 前复权日线 %d 根, 最近5根:\n", code, len(qfq))
	for _, k := range qfq[len(qfq)-5:] {
		fmt.Printf("  %s 开%.2f 高%.2f 低%.2f 收%.2f\n",
			k.Time.Format("2006-01-02"), k.Open.Float64(), k.High.Float64(), k.Low.Float64(), k.Close.Float64())
	}

	// 方式二: 已有不复权 K 线时, 直接复权
	result, err := c.GetKlineDayAll(code)
	logs.PanicErr(err)
	resp, err := result.StandardResponse()
	logs.PanicErr(err)
	hfq := gb.HFQ(code, resp.List)
	fmt.Printf("\n后复权最新: %s 收%.2f\n", hfq[len(hfq)-1].Time.Format("2006-01-02"), hfq[len(hfq)-1].Close.Float64())

	// 方式三: 仅要复权因子, 自行施加(Factor.QFQPrice / HFQPrice)
	fs := gb.GetFactors(code, resp.List)
	fmt.Printf("复权因子 %d 个(每个除权区间一个仿射系数 QFQMul/QFQAdd)\n", len(fs))
}
