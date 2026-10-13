package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// 真实连接通达信服务器，演示 report file(0x06B9) 文件下载能力：
//  1. 下载板块/配置数据总包 zhb.zip，原始字节存 output/dump/zhb.zip
//  2. 解压 zhb.zip，全部成分文件存 output/dump/zhb/<file>
//  3. 解析 tdxzs.cfg(板块指数代码 id) → output/dump/tdxzs.parsed.txt
//  4. 下载 block_*.dat 板块成分，按名称用 tdxzs.cfg 回填板块 id → output/dump/<block>.withid.txt
func main() {
	dir := "./output/dump"
	logs.PanicErr(os.MkdirAll(filepath.Join(dir, "zhb"), 0755))

	c, err := tdx.DialDefault()
	logs.PanicErr(err)
	defer c.Close()

	// 1. 下载 zhb.zip 原始包
	raw, err := c.GetReportFile(protocol.ReportZHB)
	logs.PanicErr(err)
	logs.PanicErr(os.WriteFile(filepath.Join(dir, protocol.ReportZHB), raw, 0644))
	logs.Infof("%s 原始包 %d 字节\n", protocol.ReportZHB, len(raw))

	// 2. 解压全部成分文件
	files, err := c.GetZHBFiles()
	logs.PanicErr(err)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		logs.PanicErr(os.WriteFile(filepath.Join(dir, "zhb", n), files[n], 0644))
	}
	logs.Infof("解压 %d 个文件 → %s/zhb/\n", len(names), dir)

	// 3. 解析 tdxzs.cfg 板块指数代码(id)
	zs := protocol.ParseTdxZs(files[protocol.FileTdxZs])
	var sb strings.Builder
	fmt.Fprintf(&sb, "tdxzs.cfg 板块指数, 共 %d 个\n\n", len(zs))
	for _, z := range zs {
		fmt.Fprintf(&sb, "id=%-8s 类型=%d/%d 名称=%s\n", z.Code, z.Type, z.SubType, z.Name)
	}
	logs.PanicErr(os.WriteFile(filepath.Join(dir, "tdxzs.parsed.txt"), []byte(sb.String()), 0644))
	logs.Infof("解析 tdxzs.cfg %d 个板块指数 → %s/tdxzs.parsed.txt\n", len(zs), dir)

	// 3b. 解析 tdxstat/tdxstat2/xgsg
	stat := protocol.ParseTdxStat(files[protocol.FileTdxStat])
	stat2 := protocol.ParseTdxStat2(files[protocol.FileTdxStat2])
	xgsg := protocol.ParseXgsg(files[protocol.FileXgsg])
	s2bk := protocol.StockBlockIndex(stat2)
	dumpStat(dir, stat, stat2, xgsg, s2bk)

	// 4. 下载板块成分并回填 id(简称→全称→id 链路)
	bk := protocol.ParseTdxBk(files[protocol.FileTdxBk])
	for _, bf := range []string{protocol.BlockFileZS, protocol.BlockFileGN, protocol.BlockFileFG} {
		blocks, err := c.GetBlockData(bf)
		if err != nil {
			logs.Err(bf, err)
			continue
		}
		direct := protocol.FillBlockIndex(blocks, zs)
		matched := protocol.FillBlockIndexAlias(blocks, zs, bk)

		var b strings.Builder
		fmt.Fprintf(&b, "%s 共 %d 个板块, 命中 id %d 个(直接 %d + tdxbk链路 %d)\n\n", bf, len(blocks), matched, direct, matched-direct)
		for _, blk := range blocks {
			fmt.Fprintf(&b, "id=%-8s 类型=%d 成分=%d 名称=%s\n  %s\n", blk.Index, blk.Type, len(blk.Codes), blk.Name, strings.Join(blk.Codes, " "))
		}
		logs.PanicErr(os.WriteFile(filepath.Join(dir, bf+".withid.txt"), []byte(b.String()), 0644))
		logs.Infof("%-14s %d 板块, id 命中 %d (直接%d+链路%d) → %s/%s.withid.txt\n", bf, len(blocks), matched, direct, matched-direct, dir, bf)
	}
}

func dumpStat(dir string, stat []*protocol.TdxStat, stat2 []*protocol.TdxStat2, xgsg []*protocol.TdxXgsg, s2bk map[string]string) {
	var b strings.Builder
	fmt.Fprintf(&b, "tdxstat.cfg 个股综合统计, 共 %d 只\n\n", len(stat))
	for i, v := range stat {
		if i >= 20 {
			break
		}
		fmt.Fprintf(&b, "%d|%s 日期=%s 全字段=%s\n", v.Market, v.Code, v.Date, strings.Join(v.Fields, "|"))
	}
	logs.PanicErr(os.WriteFile(filepath.Join(dir, "tdxstat.parsed.txt"), []byte(b.String()), 0644))

	b.Reset()
	fmt.Fprintf(&b, "tdxstat2.cfg 资金流向+板块归属, 共 %d 只\n\n", len(stat2))
	for i, v := range stat2 {
		if i >= 20 {
			break
		}
		fmt.Fprintf(&b, "%d|%s 日期=%s 所属板块id=%s\n", v.Market, v.Code, v.Date, v.BlockIndex)
	}
	logs.PanicErr(os.WriteFile(filepath.Join(dir, "tdxstat2.parsed.txt"), []byte(b.String()), 0644))
	logs.Infof("tdxstat %d 只, tdxstat2 %d 只, 股→板块映射 %d 条\n", len(stat), len(stat2), len(s2bk))

	b.Reset()
	fmt.Fprintf(&b, "xgsg.cfg 新股申购, 共 %d 只\n\n", len(xgsg))
	for _, v := range xgsg {
		fmt.Fprintf(&b, "%d|%s %s 申购日=%s 发行价=%.3f\n", v.Market, v.Code, v.Name, v.Date, v.IssuePrice)
	}
	logs.PanicErr(os.WriteFile(filepath.Join(dir, "xgsg.parsed.txt"), []byte(b.String()), 0644))
	logs.Infof("xgsg %d 只新股申购 → %s/xgsg.parsed.txt\n", len(xgsg), dir)
}
