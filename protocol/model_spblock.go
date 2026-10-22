package protocol

import "strings"

// FileSpBlock 专业/自定义板块成分文件名(随 zhb.zip 下发)。
//
// 与 block_*.dat(二进制定长, 每板块成分上限 400)不同, spblock.dat 是 GBK 文本, 无成分数量上限,
// 因此承载了 block_zs.dat 无法容纳的大型指数成分, 如 中证2000/中证1000/中证500/中证A500/国证2000 等。
const FileSpBlock = "spblock.dat"

// SpBlock 一个专业板块及其成分(来自 spblock.dat)。
// Codes 为 7 字符, 首字符为市场标志: 0=深 1=沪 2=北; 其后 6 位为证券代码。
// 与 Block 的 Codes 编码一致, 便于统一处理。
type SpBlock struct {
	Name  string   // 板块名称(已去除文件中的 '#' 前缀), 如 "中证2000"
	Codes []string // 成分, 7 字符 "市场+代码", 如 "0000011"(深000011)
}

// ParseSpBlock 解析 spblock.dat(GBK 文本) → 专业板块列表。
//
// 文件格式(CRLF 分行):
//
//	#板块名          ← 段头, '#' 前缀 + GBK 名称
//	0000011         ← 成分, 7 位数字 市场(1)+代码(6)
//	0000014
//	#下一个板块名
//	...
//
// 段头行以 '#' 开头; 成分行为 7 位纯数字; 其余(空行等)忽略。
func ParseSpBlock(data []byte) []*SpBlock {
	lines := strings.Split(string(GBKToUTF8(data)), "\n")
	out := make([]*SpBlock, 0, 32)
	var cur *SpBlock
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		ln = strings.Trim(ln, "\x00")
		if ln == "" {
			continue
		}
		if strings.HasPrefix(ln, "#") {
			cur = &SpBlock{Name: strings.TrimSpace(ln[1:])}
			out = append(out, cur)
			continue
		}
		if cur != nil && isSpCode(ln) {
			cur.Codes = append(cur.Codes, ln)
		}
	}
	return out
}

// isSpCode 判断是否为 spblock 成分行(7 位纯数字)。
func isSpCode(s string) bool {
	if len(s) != 7 {
		return false
	}
	for i := 0; i < 7; i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// SpBlockByName 从 ParseSpBlock 结果中按名称取板块, 未命中返回 nil。
func SpBlockByName(blocks []*SpBlock, name string) *SpBlock {
	for _, b := range blocks {
		if b.Name == name {
			return b
		}
	}
	return nil
}
