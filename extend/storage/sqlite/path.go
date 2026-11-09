package sqlite

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// validateCode 只接收不会被路径清理或斜线替换改变身份的代码。
func validateCode(c model.Code) error {
	market := c.Market.String()
	if !filepath.IsLocal(market) || filepath.ToSlash(filepath.Clean(market)) != market {
		return fmt.Errorf("sqlite: 无效市场路径 %q", market)
	}
	for _, part := range strings.Split(market, "/") {
		if !safeName(part) || strings.Contains(part, ".") {
			return fmt.Errorf("sqlite: 无效市场路径 %q", market)
		}
	}
	if c.Market == protocol.MarketFuture {
		i, err := protocol.ParseInstrument(c.Code)
		if err == nil {
			m, marketErr := i.Market()
			if marketErr == nil && m == c.Market && i.Exchange.String()+"/"+i.Code == c.Code {
				return nil
			}
		}
		return fmt.Errorf("sqlite: 期货代码须为规范的交易所/合约形式: %q", c.Code)
	}
	if !safeName(c.Code) {
		return fmt.Errorf("sqlite: 代码不能安全映射为文件名: %q", c.Code)
	}
	return nil
}

func safeName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.TrimRight(name, ". ") != name {
		return false
	}
	for _, r := range name {
		if r < 32 || strings.ContainsRune(`/\:*?"<>|[]`, r) {
			return false
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(name), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

func validateMinuteCode(c model.Code, year int) error {
	if err := validateCode(c); err != nil {
		return err
	}
	if year < 1 || year > 9999 {
		return fmt.Errorf("sqlite: 无效分钟年份 %d", year)
	}
	return nil
}

// DayFile 该代码的日线 sqlite 文件路径。
// 布局：{dir}/{编码市场层级}/day/{编码代码}.db。
func dayFile(c model.Code, dir string) string {
	if validateCode(c) != nil {
		return ""
	}
	return filepath.Join(dir, marketPath(c.Market), "day", fileName(strings.ReplaceAll(c.Code, "/", "-"))+".db")
}

// MinuteFile 该代码指定年份的1分钟线 sqlite 文件路径。
// 布局：{dir}/{编码市场层级}/min/{编码代码}/{编码代码}-{year}.db。
func minuteFile(c model.Code, dir string, year int) string {
	if validateMinuteCode(c, year) != nil {
		return ""
	}
	y := yearToString(year)
	name := fileName(strings.ReplaceAll(c.Code, "/", "-"))
	return filepath.Join(dir, marketPath(c.Market), "min", name, name+"-"+y+".db")
}

func marketPath(market protocol.Market) string {
	parts := strings.Split(market.String(), "/")
	for i, part := range parts {
		parts[i] = fileName(part)
	}
	return filepath.Join(parts...)
}

// fileName uses only lowercase ASCII. Escaping '~' itself makes the mapping
// injective even on case-insensitive or Unicode-normalizing filesystems.
func fileName(name string) string {
	const hex = "0123456789abcdef"
	var out strings.Builder
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '_' || b == '.' {
			out.WriteByte(b)
		} else {
			out.WriteByte('~')
			out.WriteByte(hex[b>>4])
			out.WriteByte(hex[b&15])
		}
	}
	return out.String()
}

func yearToString(year int) string {
	buf := []byte("0000")
	buf[0] = byte('0' + (year/1000)%10)
	buf[1] = byte('0' + (year/100)%10)
	buf[2] = byte('0' + (year/10)%10)
	buf[3] = byte('0' + year%10)
	return string(buf)
}
