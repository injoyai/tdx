package main

import (
	"fmt"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// 演示财务/基本面(GetFinanceInfo) 与 F10 公司资料(GetCompanyCategory/Content)。
func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	code := "600519"

	// 1. 财务/基本面: 流通/总股本、上市日期、股东户数、净利润等
	fi, err := c.GetFinanceInfo(protocol.ExchangeSH, code)
	logs.PanicErr(err)
	fmt.Printf("%s 流通股本=%.0f 总股本=%.0f 上市日=%d 股东户数=%.0f 净利润=%.2f\n",
		code, fi.LiuTongGuBen, fi.ZongGuBen, fi.IPODate, fi.GuDongRenShu, fi.JingLiRun)

	// 2. F10 资料分类列表
	cats, err := c.GetCompanyCategory(protocol.ExchangeSH, code)
	logs.PanicErr(err)
	fmt.Printf("\nF10 分类 %d 个:\n", len(cats))
	for _, cat := range cats {
		fmt.Printf("  %s (%s 偏移%d 长度%d)\n", cat.Name, cat.Filename, cat.Start, cat.Length)
	}

	// 3. 读取第一个分类的正文内容
	if len(cats) > 0 {
		cat := cats[0]
		content, err := c.GetCompanyContent(protocol.ExchangeSH, code, cat.Filename, cat.Start, cat.Length)
		logs.PanicErr(err)
		if r := []rune(content); len(r) > 100 { // 按字符截断, 避免切断多字节
			content = string(r[:100]) + "..."
		}
		fmt.Printf("\n【%s】正文:\n%s\n", cat.Name, content)
	}
}
