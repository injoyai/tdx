package pull

import (
	"fmt"
	"sort"
	"strings"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// 最长市场名优先，避免将 hk/index 等完整市场名拆成 hk；前缀不区分大小写。
func parseMarketPrefix(s string, resolve func(protocol.InstrumentLookup) (protocol.InstrumentSnapshot, error)) (model.Code, bool, error) {
	var prefixes []string
	wirePrefixes := map[string]bool{}
	for _, m := range protocol.Markets() {
		info, _ := m.Info()
		prefix := info.Prefix
		if !wirePrefixes[prefix] {
			wirePrefixes[prefix] = true
			prefixes = append(prefixes, prefix)
		}
		if m.String() != prefix {
			prefixes = append(prefixes, m.String())
		}
	}
	for _, u := range Units() {
		if _, builtin := protocol.MarketID(u.Name()).Exchange(); !builtin {
			prefixes = append(prefixes, u.Name())
		}
	}
	sort.SliceStable(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if len(s) <= len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
			continue
		}
		body := s[len(prefix):]
		if wirePrefixes[prefix] {
			code, err := parseWireCode(s, resolve)
			_, shared := protocol.LookupInstrument(s)
			if err == nil || shared || strings.HasPrefix(body, ".") || strings.HasPrefix(body, "/") {
				return code, true, err
			}
			continue
		}
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
	market := protocol.MarketID(prefix)
	if _, builtin := market.Exchange(); builtin {
		i, err := market.ParseInstrument(body)
		if err != nil {
			return model.Code{}, err
		}
		return model.Code{Market: market, Code: i.StorageCode()}, nil
	}
	if body == "" {
		return model.Code{}, fmt.Errorf("pull: 市场 %s 缺少代码", prefix)
	}
	return model.Code{Market: market, Code: body}, nil
}

// ParseCode 将用户输入的代码字符串自动路由为 Code（识别所属市场）。
// 支持的输入形式（宽松匹配，便于 Config.Codes 直接写裸代码）：
//
//	sz000001 / sh.600000 / bj920001     带交易所前缀，可直接拼接或用点分隔；前缀不区分大小写
//	600000                               6 位裸数字，自动补交易所前缀后再分类
//	hk.HSI                               港股指数
//	usAAPL / us.BRK.B / AAPL              美股
//	cffIF2609 / cff.IF2609 / cff/IF2609   期货
//	wi.A_IXIC / global/index.A_IXIC      国际指数
//	hk/stock.00700 / hk/fund.02800 / hk/index.HSI  完整市场键
//
// hk.00700、hk02800、00700 等五位数字港股可按已导入或内置的归属离线解析。
// 未收录的代码需要使用 ParseCodeWithSnapshot 或已加载品种快照的 Service.ParseCode。
// 无法识别时返回错误。
func ParseCode(s string) (model.Code, error) {
	return ParseCodeWithSnapshot(s, protocol.InstrumentSnapshot{})
}

// ParseCodeWithSnapshot resolves shared market symbols against an instrument snapshot.
func ParseCodeWithSnapshot(s string, snapshot protocol.InstrumentSnapshot) (model.Code, error) {
	return parseCode(s, func(protocol.InstrumentLookup) (protocol.InstrumentSnapshot, error) { return snapshot, nil })
}

func parseCode(s string, resolve func(protocol.InstrumentLookup) (protocol.InstrumentSnapshot, error)) (model.Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return model.Code{}, fmt.Errorf("pull: 空代码")
	}
	if code, ok, err := parseMarketPrefix(s, resolve); ok {
		return code, err
	}
	return parseWireCode(s, resolve)
}

func parseWireCode(s string, resolve func(protocol.InstrumentLookup) (protocol.InstrumentSnapshot, error)) (model.Code, error) {
	i, err := protocol.ResolveInstrument(s, resolve)
	if err != nil {
		return model.Code{}, err
	}
	market, err := i.Market()
	if err != nil {
		return model.Code{}, err
	}
	return model.Code{Market: market, Code: i.StorageCode()}, nil
}
