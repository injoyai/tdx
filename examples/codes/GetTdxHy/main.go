package main

import (
	"fmt"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

// 演示行业归属(通达信新行业/申万行业)。
// GetTdxHy 下载 tdxhy.cfg(全市场)并解析: 每只股票对应通达信行业码(T前缀)与申万行业码(X前缀)。
func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	hy, err := c.GetTdxHy()
	logs.PanicErr(err)
	fmt.Printf("行业归属共 %d 只\n", len(hy))

	// 查几只样本
	want := map[string]bool{"600519": true, "000001": true, "601398": true}
	for _, v := range hy {
		if want[v.Code] {
			fmt.Printf("  %d|%s 通达信行业=%s 申万行业=%s\n", v.Market, v.Code, v.TdxHy, v.SwHy)
		}
	}
}
