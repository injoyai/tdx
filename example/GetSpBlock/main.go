package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// 下载 spblock.dat(专业板块)并打印大型指数成分。
//
// spblock.dat 承载 block_zs.dat 无法容纳的成分(block_*.dat 单板块上限 400):
// 中证2000/中证1000/中证500/中证A500/国证2000 等。
// 沪深300 不在此文件, 在 block_zs.dat, 用 c.GetBlockData(protocol.BlockFileZS) 获取。
func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	// 1. 专业板块(中证2000/1000/500 等)
	blocks, err := c.GetSpBlock()
	logs.PanicErr(err)
	logs.Infof("spblock.dat 共 %d 个专业板块\n", len(blocks))
	for _, b := range blocks {
		logs.Infof("板块=%s 成分数=%d\n", b.Name, len(b.Codes))
	}

	// 2. 取中证2000成分(前10)
	if b := protocol.SpBlockByName(blocks, "中证2000"); b != nil {
		logs.Infof("中证2000 成分数=%d, 前10=%v\n", len(b.Codes), first(b.Codes, 10))
	}

	// 3. 沪深300 走 block_zs.dat
	zs, err := c.GetBlockData(protocol.BlockFileZS)
	logs.PanicErr(err)
	for _, b := range zs {
		if b.Name == "沪深300" {
			logs.Infof("沪深300 成分数=%d, 前10=%v\n", len(b.Codes), first(b.Codes, 10))
		}
	}
}

func first(s []string, n int) []string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
