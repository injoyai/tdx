package pull

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/injoyai/tdx/protocol"
)

// Code 统一的代码模型，跨市场通用。
type Code struct {
	Market Market // 市场标识，如 MarketAStock/MarketHK/MarketUS/MarketFuture；用于定位 Unit 与存储目录
	Code   string // 原始代码，如 600000 / 00700 / AAPL / IF2609
	Name   string // 名称（可选）
}

// Key 返回带完整市场前缀的唯一键，如 sh600000 / HK00700 / US.AAPL / future.IF2609。
// 该键同时用于存储文件名，天然避免跨市场撞名。
func (c Code) Key() string {
	switch c.Market {
	case MarketAStock, MarketIndex, MarketEtfLof, MarketBlock:
		// 标准行情代码自带交易所前缀（sh/sz/bj），直接使用
		return c.Code
	case MarketHK:
		return "HK" + c.Code
	case MarketUS:
		return "US." + c.Code
	default:
		return c.Market.String() + "." + c.Code
	}
}

// DayFile 该代码的日线 sqlite 文件路径。
func (c Code) DayFile(dir string) string {
	return filepath.Join(dir, "day", c.Key()+".db")
}

// MinFile 该代码指定年份的1分钟线 sqlite 文件路径。
func (c Code) MinFile(dir string, year int) string {
	return filepath.Join(dir, "min", c.Key(), yearToString(year)+".db")
}

func yearToString(year int) string {
	buf := []byte("0000")
	buf[0] = byte('0' + (year/1000)%10)
	buf[1] = byte('0' + (year/100)%10)
	buf[2] = byte('0' + (year/10)%10)
	buf[3] = byte('0' + year%10)
	return string(buf)
}

// SplitKey 从完整键还原 Market 与 Code；无法识别时返回原样。
func SplitKey(key string) Code {
	if strings.HasPrefix(key, "US.") {
		return Code{Market: MarketUS, Code: strings.TrimPrefix(key, "US.")}
	}
	if strings.HasPrefix(key, "HK") && len(key) > 2 {
		return Code{Market: MarketHK, Code: strings.TrimPrefix(key, "HK")}
	}
	if i := strings.IndexByte(key, '.'); i > 0 {
		return Code{Market: Market(key[:i]), Code: key[i+1:]}
	}
	return Code{Code: key}
}

// ParseCode 将用户输入的代码字符串自动路由为 Code（识别所属市场）。
// 支持的输入形式（宽松匹配，便于 PullConfig.Codes 直接写裸代码）：
//
//	sz000001 / sh600000 / bj920001    带交易所前缀，自动分类为 a_stock / etf_lof / index / block
//	600000                             6 位裸数字，自动补交易所前缀后再分类
//	HK00700 / 00700                    港股（5 位数字）
//	AAPL                               美股（纯字母）
//	cff/IF2609                         期货（"交易所/" 前缀）
//	future.IF2609 / hk.00700 / US.AAPL 完整键（SplitKey 也可解析）
//
// 无法识别时返回错误。
func ParseCode(s string) (Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Code{}, fmt.Errorf("pull: 空代码")
	}
	// 完整键（market.code 或 US. 前缀）优先
	if i := strings.IndexByte(s, '.'); i > 0 {
		m := Market(s[:i])
		switch m {
		case MarketAStock, MarketIndex, MarketEtfLof, MarketBlock:
			return Code{Market: m, Code: s[i+1:]}, nil
		default:
			c := SplitKey(s)
			if c.Market != "" {
				return c, nil
			}
		}
	}
	// 期货：交易所前缀 "cff/IF2609"
	if i := strings.IndexByte(s, '/'); i > 0 {
		return Code{Market: MarketFuture, Code: s}, nil
	}
	// 带交易所前缀的 8 位标准行情代码（sh600000/sz000001/bj920001/sh880001...）
	if len(s) == 8 {
		m, ok := classifyStd(s)
		if ok {
			return Code{Market: m, Code: strings.ToLower(s)}, nil
		}
	}
	// 6 位裸数字：补交易所前缀后分类（AddPrefix 已覆盖股票/ETF/指数/板块/债券）
	if len(s) == 6 && isAllDigit(s) {
		p := protocol.AddPrefix(s)
		if p != s {
			if m, ok := classifyStd(p); ok {
				return Code{Market: m, Code: p}, nil
			}
		}
		// 6 位数字但补不上前缀（如北交所指数 899xxx）：按北交所处理
		if strings.HasPrefix(s, "89") {
			return Code{Market: MarketIndex, Code: "bj" + s}, nil
		}
	}
	// 港股：HK 前缀或 5 位数字
	if strings.HasPrefix(s, "HK") && isAllDigit(s[2:]) && len(s) == 7 {
		return Code{Market: MarketHK, Code: s[2:]}, nil
	}
	if len(s) == 5 && isAllDigit(s) {
		return Code{Market: MarketHK, Code: s}, nil
	}
	// 美股：纯字母
	if isAllLetter(s) {
		return Code{Market: MarketUS, Code: s}, nil
	}
	return Code{}, fmt.Errorf("pull: 无法识别代码 %q 的所属市场", s)
}

// classifyStd 将带交易所前缀的标准行情代码（sh/sz/bj + 6 位）分类到 Market。
// 优先级：ETF > 股票 > 板块 > 指数（协议谓词区间互斥，实际不会重叠）。
func classifyStd(code string) (Market, bool) {
	switch {
	case protocol.IsETF(code):
		return MarketEtfLof, true
	case protocol.IsStock(code):
		return MarketAStock, true
	case isBlockCode(code):
		return MarketBlock, true
	case protocol.IsIndex(code):
		return MarketIndex, true
	}
	return "", false
}

// isBlockCode 板块指数 880xxx/881xxx（与 protocol.isBlock 同规则，该函数未导出）。
func isBlockCode(code string) bool {
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	if code[:2] != "sh" {
		return false
	}
	n := code[2:]
	return n[:3] == "880" || n[:3] == "881"
}

func isAllDigit(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isAllLetter(s string) bool {
	for _, c := range s {
		if c < 'A' || c > 'z' || (c > 'Z' && c < 'a') {
			return false
		}
	}
	return len(s) > 0
}
