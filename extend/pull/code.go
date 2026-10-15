package pull

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/injoyai/tdx/v2/protocol"
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
	lower := strings.ToLower(prefix)
	query := lower + "." + body
	explicit := Market(lower)
	switch explicit {
	case MarketAStock, MarketIndex, MarketEtfLof, MarketBlock, MarketFuture:
		query = body
	case MarketHK:
		query = "hk." + body
	case MarketHKIndex:
		query = "hi." + body
	case MarketUS:
		query = "us." + body
	default:
		switch lower {
		case "sh", "sz", "bj", "hk", "us", "cff", "czc", "dce", "shf", "gfe", "qhz":
			explicit = ""
		default:
			if body == "" {
				return Code{}, fmt.Errorf("pull: 市场 %s 缺少代码", prefix)
			}
			return Code{Market: Market(prefix), Code: body}, nil
		}
	}
	i, err := protocol.ParseInstrument(query)
	if err != nil {
		return Code{}, err
	}
	market, err := i.Market()
	if err != nil {
		return Code{}, err
	}
	code := i.Code
	switch market {
	case MarketAStock, MarketIndex, MarketEtfLof, MarketBlock:
		code = i.Exchange.String() + i.Code
		if market == MarketAStock && !protocol.IsStock(code) {
			return Code{}, fmt.Errorf("pull: 不支持的标准行情品种 %q", body)
		}
		if explicit != "" {
			// 完整市场键沿用已有文件名，避免裸数字旧库被另建一份。
			code = strings.ToLower(body)
		}
	case MarketFuture:
		code = i.Exchange.String() + "/" + i.Code
	}
	if explicit != "" && explicit != market {
		return Code{}, fmt.Errorf("pull: 代码 %q 不属于市场 %s", body, prefix)
	}
	return Code{Market: market, Code: code}, nil
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
	if len(s) == 6 && isAllDigit(s) {
		p := protocol.AddPrefix(s)
		if p != s {
			return codeInMarket(p[:2], p[2:])
		}
		if strings.HasPrefix(s, "89") {
			return codeInMarket("bj", s)
		}
	}
	if len(s) == 5 && isAllDigit(s) {
		return codeInMarket("hk", s)
	}
	if isAllLetter(s) {
		return codeInMarket("us", s)
	}
	return Code{}, fmt.Errorf("pull: 无法识别代码 %q 的所属市场", s)
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
