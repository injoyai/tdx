package pull

import (
	"path/filepath"
	"strings"
)

// Code 统一的代码模型，跨市场通用。
type Code struct {
	Market string // 市场标识，如 a_stock/hk/us/future；用于定位 Unit 与存储目录
	Code   string // 原始代码，如 600000 / 00700 / AAPL / IF2609
	Name   string // 名称（可选）
}

// Key 返回带完整市场前缀的唯一键，如 sh600000 / HK00700 / US.AAPL / future.IF2609。
// 该键同时用于存储文件名，天然避免跨市场撞名。
func (c Code) Key() string {
	switch c.Market {
	case "a_stock", "index", "etf_lof", "block":
		// 标准行情代码自带交易所前缀（sh/sz/bj），直接使用
		return c.Code
	case "hk":
		return "HK" + c.Code
	case "us":
		return "US." + c.Code
	default:
		return c.Market + "." + c.Code
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
		return Code{Market: "us", Code: strings.TrimPrefix(key, "US.")}
	}
	if strings.HasPrefix(key, "HK") && len(key) > 2 {
		return Code{Market: "hk", Code: strings.TrimPrefix(key, "HK")}
	}
	if i := strings.IndexByte(key, '.'); i > 0 {
		return Code{Market: key[:i], Code: key[i+1:]}
	}
	return Code{Code: key}
}
