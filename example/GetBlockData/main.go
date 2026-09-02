package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

// 拉取通达信全量板块数据(概念/风格/行业/指数/专业板块), 并演示板块指数(880xxx)行情。
//
// 板块数据来源:
//   - block_gn.dat 概念板块      -> c.GetBlockDataWithIndex(BlockFileGN), 含指数代码(880xxx)
//   - block_fg.dat 风格板块(含地域) -> c.GetBlockDataWithIndex(BlockFileFG), 含指数代码(881xxx)
//   - block_hy.dat 行业板块       -> c.GetBlockDataWithIndex(BlockFileHY), 含指数代码(880xxx)
//   - block_zs.dat 指数板块(沪深300等) -> c.GetBlockData(BlockFileZS), 无指数代码
//   - spblock.dat 专业板块(中证2000/1000/500等) -> c.GetSpBlock()
//
// 板块指数代码(880xxx/881xxx)归属上海交易所(ExchangeSH), 可用 GetIndexDay/GetQuote 直接获取行情。
func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	// 1. 概念板块(含板块指数代码)
	gn, err := c.GetBlockDataWithIndex(protocol.BlockFileGN)
	logs.PanicErr(err)
	logs.Infof("概念板块共 %d 个, 示例:", len(gn))
	for _, b := range gn[:5] {
		logs.Infof("  板块=%s 指数=%s 成分数=%d 前5=%v", b.Name, b.Index, len(b.Codes), first(b.Codes, 5))
	}

	// 2. 风格板块(含地域, 指数代码 881xxx)
	fg, err := c.GetBlockDataWithIndex(protocol.BlockFileFG)
	logs.PanicErr(err)
	logs.Infof("风格板块共 %d 个, 示例:", len(fg))
	for _, b := range fg[:5] {
		logs.Infof("  板块=%s 指数=%s 成分数=%d", b.Name, b.Index, len(b.Codes))
	}

	// 3. 行业板块
	hy, err := c.GetBlockDataWithIndex(protocol.BlockFileHY)
	if err != nil {
		logs.Warnf("行业板块不可用: %v", err)
	} else {
		logs.Infof("行业板块共 %d 个, 示例:", len(hy))
		for _, b := range hy[:5] {
			logs.Infof("  板块=%s 指数=%s 成分数=%d", b.Name, b.Index, len(b.Codes))
		}
	}

	// 4. 指数板块(沪深300 等, 无指数代码)
	zs, err := c.GetBlockData(protocol.BlockFileZS)
	logs.PanicErr(err)
	logs.Infof("指数板块共 %d 个", len(zs))
	for _, b := range zs {
		if b.Name == "沪深300" {
			logs.Infof("  沪深300 成分数=%d 前5=%v", len(b.Codes), first(b.Codes, 5))
		}
	}

	// 5. 专业板块(中证2000/1000/500 等)
	sp, err := c.GetSpBlock()
	logs.PanicErr(err)
	logs.Infof("专业板块共 %d 个, 示例:", len(sp))
	for _, b := range sp[:5] {
		logs.Infof("  板块=%s 成分数=%d", b.Name, len(b.Codes))
	}

	// 6. 板块指数行情(取第1个概念板块的指数代码)
	if len(gn) > 0 && gn[0].Index != "" {
		code := gn[0].Index
		ks, err := c.GetIndexDay(code, 0, 5)
		logs.PanicErr(err)
		logs.Infof("板块[%s]指数 %s 最近5个交易日:", gn[0].Name, code)
		for _, v := range ks.List {
			logs.Infof("  %s 收=%.3f 涨跌家数=%d/%d 量=%d", v.Time.Format("2006-01-02"), v.Close.Float64(), v.UpCount, v.DownCount, v.Volume)
		}
		q, err := c.GetQuote(code)
		logs.PanicErr(err)
		for _, v := range q {
			logs.Infof("  实时行情 %s%s: 收=%.3f 昨收=%.3f", v.Exchange.String(), v.Code, v.Kline.Close.Float64(), v.Kline.Last.Float64())
		}
	}
}

func first(s []string, n int) []string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
