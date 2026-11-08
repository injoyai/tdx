package pull

import (
	"fmt"
	"sort"
	"strings"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

var marketPrefixes = []string{
	protocol.MarketAStock.String(), protocol.MarketIndex.String(), protocol.MarketEtfLof.String(), protocol.MarketBlock.String(),
	protocol.MarketFuture.String(), protocol.MarketHK.String(), protocol.MarketHKIndex.String(), protocol.MarketUS.String(),
	protocol.MarketBond.String(), protocol.MarketOther.String(),
}

// 最长市场名优先，避免将 hk/index 等完整市场名拆成 hk；前缀不区分大小写。
func parseMarketPrefix(s string) (model.Code, bool, error) {
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
	return model.Code{}, false, nil
}

func codeInMarket(prefix, body string) (model.Code, error) {
	lower := strings.ToLower(prefix)
	query := lower + "." + body
	explicit := protocol.Market(lower)
	switch explicit {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock, protocol.MarketBond, protocol.MarketOther, protocol.MarketFuture:
		query = body
	case protocol.MarketHK:
		query = "hk." + body
	case protocol.MarketHKIndex:
		query = "hi." + body
	case protocol.MarketUS:
		query = "us." + body
	default:
		if body == "" {
			return model.Code{}, fmt.Errorf("pull: 市场 %s 缺少代码", prefix)
		}
		return model.Code{Market: protocol.Market(prefix), Code: body}, nil
	}
	i, err := protocol.ParseInstrument(query)
	if err != nil {
		return model.Code{}, err
	}
	return codeForInstrument(i, explicit, body)
}

func codeForInstrument(i protocol.Instrument, explicit protocol.Market, body string) (model.Code, error) {
	market, err := i.Market()
	if err != nil {
		return model.Code{}, err
	}
	code := i.Code
	switch market {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock, protocol.MarketBond, protocol.MarketOther:
		code = i.Exchange.String() + i.Code
	case protocol.MarketFuture:
		code = i.Exchange.String() + "/" + i.Code
	}
	if explicit != "" && explicit != market {
		return model.Code{}, fmt.Errorf("pull: 代码 %q 不属于市场 %s", body, explicit)
	}
	return model.Code{Market: market, Code: code}, nil
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
func ParseCode(s string) (model.Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return model.Code{}, fmt.Errorf("pull: 空代码")
	}
	if code, ok, err := parseMarketPrefix(s); ok {
		return code, err
	}
	i, err := protocol.ParseInstrument(s)
	if err != nil {
		return model.Code{}, err
	}
	return codeForInstrument(i, "", s)
}
