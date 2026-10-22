package protocol

import "strings"

// FileTdxBk 概念板块简称↔全称配置文件名(随 zhb.zip 下发)。
// 已在 model_block.go 声明为 FileTdxBk 常量, 此处不重复。

// TdxBk 概念板块简称↔全称(来自 tdxbk.cfg, 4 字段: flag|简称|全称|flag2)。
//
// block_gn.dat 板块名用简称(如 "锂电池"), tdxzs.cfg 板块指数用全称(如 "锂电池概念"),
// 故 tdxbk 是两者关联的中间桥: 简称 →(tdxbk)→ 全称 →(tdxzs)→ 板块指数代码(id)。
type TdxBk struct {
	Short string // 板块简称(block_*.dat 中使用)
	Full  string // 板块全称(tdxzs.cfg 中使用)
}

// ParseTdxBk 解析 tdxbk.cfg(GBK 文本) → 简称↔全称列表。
// 每行 `flag|简称|全称|flag2`，例: `1|锂电池|锂电池概念|0`。
func ParseTdxBk(data []byte) []*TdxBk {
	lines := strings.Split(string(GBKToUTF8(data)), "\n")
	out := make([]*TdxBk, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Split(ln, "|")
		if len(f) < 3 || f[1] == "" || f[2] == "" {
			continue
		}
		out = append(out, &TdxBk{Short: f[1], Full: f[2]})
	}
	return out
}

// FillBlockIndexAlias 在按名称回填板块指数代码(id)的基础上，
// 对直接未命中的板块用 tdxbk 简称→全称做二次匹配，提升命中率。返回命中数。
// bk 传 nil 时退化为等同 FillBlockIndex。
func FillBlockIndexAlias(blocks []*Block, zs []*TdxZs, bk []*TdxBk) int {
	nameToCode := make(map[string]string, len(zs))
	for _, z := range zs {
		nameToCode[z.Name] = z.Code
	}
	shortToFull := make(map[string]string, len(bk))
	for _, b := range bk {
		shortToFull[b.Short] = b.Full
	}
	n := 0
	for _, blk := range blocks {
		code, ok := nameToCode[blk.Name]
		if !ok {
			if full, has := shortToFull[blk.Name]; has {
				code, ok = nameToCode[full]
			}
		}
		if ok {
			blk.Index = code
			n++
		}
	}
	return n
}
