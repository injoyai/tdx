package protocol

import "strings"

// FileXgsg 新股申购配置文件名(随 zhb.zip 下发)。
const FileXgsg = "xgsg.cfg"

// TdxXgsg 新股申购信息(来自 xgsg.cfg, 18 字段, GBK 文本, `|` 分隔)。
//
// Market/Code/Date/Name/IssuePrice 字段语义确定; 其余(发行量/中签率/募资额/首日涨幅限制等)
// 为通达信内部格式、无官方文档, 命名按数值量级/位置推断, 完整原始字段保留在 Fields(0 基)。
// 经验推断(未核验): [4]发行量(万股) [5]网上发行量 [6]首日涨幅限制% [9]中签率 [11]募资/总市值。
type TdxXgsg struct {
	Market     uint8    // [0] 市场 0=深 1=沪 2=京
	Code       string   // [1] 申购代码
	Date       string   // [2] 申购日期 YYYYMMDD
	IssuePrice float64  // [3] 发行价
	Name       string   // [14] 证券名称
	Fields     []string // 全部 18 个原始字段
}

// ParseXgsg 解析 xgsg.cfg → 新股申购列表。
func ParseXgsg(data []byte) []*TdxXgsg {
	lines := strings.Split(string(GBKToUTF8(data)), "\n")
	out := make([]*TdxXgsg, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Split(ln, "|")
		if len(f) < 15 || f[1] == "" {
			continue
		}
		out = append(out, &TdxXgsg{
			Market:     uint8(Uint16FromStr(f[0])),
			Code:       f[1],
			Date:       field(f, 2),
			IssuePrice: Float64FromStr(field(f, 3)),
			Name:       field(f, 14),
			Fields:     f,
		})
	}
	return out
}
