package main

import (
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// 下载概念板块成分并回填板块指数代码(id, 经 tdxzs.cfg + tdxbk.cfg 关联)。
func main() {
	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	blocks, err := c.GetBlockDataWithIndex(protocol.BlockFileGN)
	logs.PanicErr(err)

	for _, b := range blocks {
		logs.Infof("板块=%s id=%s 类型=%d 成分数=%d\n", b.Name, b.Index, b.Type, len(b.Codes))
	}
}
