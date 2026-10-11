package pull

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/injoyai/tdx/protocol"
)

// Code 统一的代码模型，跨市场通用。
type Code struct {
	Market Market // 市场标识，如 MarketAStock/MarketHK/MarketUS/MarketFuture；用于定位 Unit 与存储目录
	Code   string // 原始代码，如 600000 / 00700 / AAPL / IF2609
	Name   string // 名称（可选）
}

// Key 返回带完整市场前缀的唯一键，如 sh600000 / HK00700 / US.AAPL /
// future.cff/IF2609 / hk/index.HSI。用于日志/进度条显示，天然避免跨市场撞名；
// 注意存储文件名不用它（见 FileName，目录已含市场信息，文件名用原始代码）。
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

// DirName 市场数据目录。枚举值本身即两级「地区/资产」路径（如 cn/stock、
// hk/index、us/stock），与磁盘布局完全一致；未知市场取枚举值原样。
func (c Code) DirName() string {
	return c.Market.String()
}

// FileName 存储文件名：直接用原始代码——目录已含市场信息，文件名无需市场前缀
// （hk/index/day/HSI.db 而非 hk-index.HSI.db）；期货多交易所代码含 "/"
// （如 cff/IF2609）替换为 "-"，避免嵌套目录。同市场内代码唯一，无撞名风险。
func (c Code) FileName() string {
	return strings.ReplaceAll(c.Code, "/", "-")
}

// DayFile 该代码的日线 sqlite 文件路径。
// 布局：{dir}/{地区}/{资产}/day/{code}.db（两级目录含市场信息，文件名用原始代码）。
func (c Code) DayFile(dir string) string {
	return filepath.Join(dir, c.DirName(), "day", c.FileName()+".db")
}

// MinFile 该代码指定年份的1分钟线 sqlite 文件路径。
// 布局：{dir}/{地区}/{资产}/min/{code}/{code}-{year}.db（子目录与文件名均用原始代码，
// 便于按代码单独检索/拷贝；期货代码中的 "/" 替换为 "-"）。
func (c Code) MinFile(dir string, year int) string {
	y := yearToString(year)
	name := c.FileName()
	return filepath.Join(dir, c.DirName(), "min", name, name+"-"+y+".db")
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
// 支持市场前缀的直接拼接和点分隔形式，以及原有的完整市场键。
func SplitKey(key string) Code {
	key = strings.TrimSpace(key)
	if code, ok, err := parseMarketPrefix(key); ok {
		if err == nil {
			return code
		}
		return Code{Code: key}
	}
	if i := strings.IndexByte(key, '.'); i > 0 {
		return Code{Market: Market(strings.ToLower(key[:i])), Code: key[i+1:]}
	}
	return Code{Code: key}
}

var marketPrefixes = []string{
	MarketAStock.String(), MarketIndex.String(), MarketEtfLof.String(), MarketBlock.String(),
	MarketFuture.String(), MarketHK.String(), MarketHKIndex.String(), MarketUS.String(),
	"sh", "sz", "bj", "hk", "us", "cff", "czc", "dce", "shf", "gfe", "qhz",
}

// 最长市场名优先，避免将 hk/index 等完整市场名拆成 hk；前缀不区分大小写。
func parseMarketPrefix(s string) (Code, bool, error) {
	// 裸 HKL 是完整指数代码，不能拆成 hk + L。
	if _, ok := hkIndexCodes[strings.ToUpper(s)]; ok {
		return Code{Market: MarketHKIndex, Code: strings.ToUpper(s)}, true, nil
	}
	prefixes := append([]string(nil), marketPrefixes...)
	for _, u := range Units() {
		prefixes = append(prefixes, u.Name())
	}
	sort.SliceStable(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if len(s) <= len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
			continue
		}
		body := s[len(prefix):]
		dotted := strings.HasPrefix(body, ".")
		lower := strings.ToLower(prefix)
		future := lower == "cff" || lower == "czc" || lower == "dce" || lower == "shf" || lower == "gfe" || lower == "qhz"
		slashed := future && strings.HasPrefix(body, "/")
		if dotted || slashed {
			body = body[1:]
		}
		code, err := codeInMarket(prefix, body)
		if err == nil || dotted || slashed || strings.Contains(prefix, "/") {
			return code, true, err
		}
		// 无分隔形式只有主体合法才拆分，保留 SHOP 等裸美股代码。
	}
	return Code{}, false, nil
}

func codeInMarket(prefix, body string) (Code, error) {
	if body == "" {
		return Code{}, fmt.Errorf("pull: 市场 %s 缺少代码", prefix)
	}
	lower := strings.ToLower(prefix)
	switch lower {
	case "sh", "sz", "bj":
		if len(body) == 6 && isAllDigit(body) {
			if market, ok := classifyStd(lower + body); ok {
				return Code{Market: market, Code: lower + body}, nil
			}
		}
	case "hk":
		body = strings.ToUpper(body)
		if len(body) == 5 && isAllDigit(body) {
			return Code{Market: MarketHK, Code: body}, nil
		}
		if isAlphanumeric(body) && !isAllDigit(body) {
			return Code{Market: MarketHKIndex, Code: body}, nil
		}
	case "us":
		for _, part := range strings.Split(strings.ReplaceAll(body, "-", "."), ".") {
			if !isAlphanumeric(part) {
				return Code{}, fmt.Errorf("pull: 非法美股代码 %q", body)
			}
		}
		return Code{Market: MarketUS, Code: strings.ToUpper(body)}, nil
	case "cff", "czc", "dce", "shf", "gfe", "qhz":
		if isAlphanumeric(body) && !isAllLetter(body) {
			return Code{Market: MarketFuture, Code: lower + "/" + body}, nil
		}
	default:
		market := Market(prefix)
		switch Market(lower) {
		case MarketAStock, MarketIndex, MarketEtfLof, MarketBlock:
			market, body = Market(lower), strings.ToLower(body)
		case MarketHK, MarketHKIndex, MarketUS:
			market, body = Market(lower), strings.ToUpper(body)
		case MarketFuture:
			market = MarketFuture
			if exchange, code, ok := strings.Cut(body, "/"); ok {
				body = strings.ToLower(exchange) + "/" + code
			}
		}
		return Code{Market: market, Code: body}, nil
	}
	return Code{}, fmt.Errorf("pull: 市场 %s 的代码 %q 无效", prefix, body)
}

// hkIndexCodes 知名港股指数白名单（扩展行情市场27实测品种）。
// 纯字母/字母数字代码与美股形态相同，需白名单区分；其余 HZ50xx/CESxxx
// 等可用 "hk.CES120" 指定。
var hkIndexCodes = map[string]struct{}{
	"HSI":    {}, // 恒生指数
	"HKL":    {}, // 大型股指数
	"VHSI":   {}, // 恒指波幅指数
	"CES100": {}, // 港股通精选100
	"GEM":    {}, // 创业板指数
}

// ParseCode 将用户输入的代码字符串自动路由为 Code（识别所属市场）。
// 支持的输入形式（宽松匹配，便于 Config.Codes 直接写裸代码）：
//
//	sz000001 / sh.600000 / bj920001     带交易所前缀，可直接拼接或用点分隔；前缀不区分大小写
//	600000                               6 位裸数字，自动补交易所前缀后再分类
//	hk00700 / hk.HSI / 00700              港股股票或指数
//	usAAPL / us.BRK.B / AAPL              美股
//	cffIF2609 / cff.IF2609 / cff/IF2609   期货（兼容原有斜线形式）
//	cn/future.cff/IF2609 / hk/stock.00700 / US.AAPL / hk/index.HSI  完整键（SplitKey 也可解析）
//
// 无法识别时返回错误。
func ParseCode(s string) (Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Code{}, fmt.Errorf("pull: 空代码")
	}
	if code, ok, err := parseMarketPrefix(s); ok {
		return code, err
	}
	// 保留自定义市场的完整键形式。
	if i := strings.IndexByte(s, '.'); i > 0 {
		return SplitKey(s), nil
	}
	// 期货：交易所前缀 "cff/IF2609"
	if i := strings.IndexByte(s, '/'); i > 0 {
		return Code{Market: MarketFuture, Code: s}, nil
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
	// 裸港股代码：5 位数字。
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

func isAlphanumeric(s string) bool {
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return s != ""
}
