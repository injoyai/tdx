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
	Market protocol.Market // 市场标识，如 protocol.MarketAStock/protocol.MarketHK/protocol.MarketUS/protocol.MarketFuture；用于定位 Unit 与存储目录
	Code   string          // 原始代码，如 600000 / 00700 / AAPL / IF2609
	Name   string          // 名称（可选）
}

// Key 返回带完整市场前缀的唯一键，如 sh600000 / HK00700 / US.AAPL /
// future.cff/IF2609 / hk/index.HSI。用于日志/进度条显示，天然避免跨市场撞名；
// 注意存储文件名不用它（见 FileName，目录已含市场信息，文件名用原始代码）。
func (c Code) Key() string {
	switch c.Market {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock:
		// 标准行情代码自带交易所前缀（sh/sz/bj），直接使用
		return c.Code
	case protocol.MarketHK:
		return "HK" + c.Code
	case protocol.MarketUS:
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

// MinuteFile 该代码指定年份的1分钟线 sqlite 文件路径。
// 布局：{dir}/{地区}/{资产}/min/{code}/{code}-{year}.db（子目录与文件名均用原始代码，
// 便于按代码单独检索/拷贝；期货代码中的 "/" 替换为 "-"）。
func (c Code) MinuteFile(dir string, year int) string {
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

var marketPrefixes = []string{
	protocol.MarketAStock.String(), protocol.MarketIndex.String(), protocol.MarketEtfLof.String(), protocol.MarketBlock.String(),
	protocol.MarketFuture.String(), protocol.MarketHK.String(), protocol.MarketHKIndex.String(), protocol.MarketUS.String(),
}

// 最长市场名优先，避免将 hk/index 等完整市场名拆成 hk；前缀不区分大小写。
func parseMarketPrefix(s string) (Code, bool, error) {
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
		if dotted {
			body = body[1:]
		}
		code, err := codeInMarket(prefix, body)
		if err == nil || dotted || strings.Contains(prefix, "/") {
			return code, true, err
		}
		// 无分隔形式只有主体合法才拆分，保留 SHOP 等裸美股代码。
	}
	return Code{}, false, nil
}

func codeInMarket(prefix, body string) (Code, error) {
	lower := strings.ToLower(prefix)
	query := lower + "." + body
	explicit := protocol.Market(lower)
	switch explicit {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock, protocol.MarketFuture:
		query = body
	case protocol.MarketHK:
		query = "hk." + body
	case protocol.MarketHKIndex:
		query = "hi." + body
	case protocol.MarketUS:
		query = "us." + body
	default:
		if body == "" {
			return Code{}, fmt.Errorf("pull: 市场 %s 缺少代码", prefix)
		}
		return Code{Market: protocol.Market(prefix), Code: body}, nil
	}
	i, err := protocol.ParseInstrument(query)
	if err != nil {
		return Code{}, err
	}
	return codeForInstrument(i, explicit, body)
}

func codeForInstrument(i protocol.Instrument, explicit protocol.Market, body string) (Code, error) {
	market, err := i.Market()
	if err != nil {
		return Code{}, err
	}
	code := i.Code
	switch market {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock:
		code = i.Exchange.String() + i.Code
		if market == protocol.MarketAStock && !protocol.IsStock(code) {
			return Code{}, fmt.Errorf("pull: 不支持的标准行情品种 %q", body)
		}
	case protocol.MarketFuture:
		code = i.Exchange.String() + "/" + i.Code
	}
	if explicit != "" && explicit != market {
		return Code{}, fmt.Errorf("pull: 代码 %q 不属于市场 %s", body, explicit)
	}
	return Code{Market: market, Code: code}, nil
}

// ParseCode 将用户输入的代码字符串自动路由为 Code（识别所属市场）。
// 支持的输入形式（宽松匹配，便于 Config.Codes 直接写裸代码）：
//
//	sz000001 / sh.600000 / bj920001     带交易所前缀，可直接拼接或用点分隔；前缀不区分大小写
//	600000                               6 位裸数字，自动补交易所前缀后再分类
//	hk00700 / hk.HSI / 00700              港股股票或指数
//	usAAPL / us.BRK.B / AAPL              美股
//	cffIF2609 / cff.IF2609 / cff/IF2609   期货
//	cn/future.cff/IF2609 / hk/stock.00700 / US.AAPL / hk/index.HSI  完整市场键
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
	i, err := protocol.ParseInstrument(s)
	if err != nil {
		return Code{}, err
	}
	return codeForInstrument(i, "", s)
}
