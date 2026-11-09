package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	if err := run(); err != nil {
		slog.Error("示例执行失败", "err", err)
		os.Exit(1)
	}
}

func run() error {
	exchange := flag.Uint("exchange", uint(protocol.ExchangeSH), "市场编码：0=深圳，1=上海，2=北京，31=港股")
	start := flag.Uint("start", 0, "市场内分页偏移，0..65535")
	count := flag.Uint("count", 20, "单页数量，1..1000")
	all := flag.Bool("all", false, "获取指定市场的完整列表")
	stocks := flag.Bool("stocks", false, "获取沪深北全部股票代码")
	flag.Parse()
	if *exchange > 255 || *start > 65535 || *count == 0 || *count > 1000 {
		return fmt.Errorf("市场或分页参数超出范围")
	}
	if *stocks && *all {
		return fmt.Errorf("-stocks 与 -all 不能同时使用")
	}
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	if *stocks {
		codes, err := c.GetStockCodeAll()
		if err != nil {
			return err
		}
		for _, code := range codes {
			fmt.Println(code)
		}
		fmt.Println("股票总数:", len(codes))
		return nil
	}
	market := protocol.Exchange(*exchange)
	total, err := c.GetCount(market)
	if err != nil {
		return err
	}
	resp, err := query(c, market, uint32(*start), uint16(*count), *all)
	if err != nil {
		return err
	}
	for _, code := range resp.List {
		fmt.Println(code.Instrument, code.Name)
	}
	fmt.Println("市场总数:", total, "本次返回:", resp.Count)
	return nil
}

func query(c tdx.Client, exchange protocol.Exchange, start uint32, count uint16, all bool) (*tdx.CodeResp, error) {
	if all {
		return c.GetCodeAll(exchange)
	}
	return c.GetCode(exchange, start, count)
}
