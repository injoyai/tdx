package market

import (
	"fmt"
	"strings"
)

// DecodeCode parses a symbol and its wire exchange. Unlike ParseInstrument,
// it also accepts dot-qualified codes for markets outside the unified Client.
func DecodeCode(code string) (Exchange, string, error) {
	i, err := parseInstrument(code, true)
	return i.Exchange, i.Code, err
}

func rawInstrument(ex Exchange, code string) (Instrument, error) {
	switch ex {
	case ExchangeSHO, ExchangeSZO, ExchangeOF, ExchangeHG, ExchangeNQ:
		if len(code) == 0 || len(code) > 6 || isStockSymbol(code) {
			return Instrument{}, fmt.Errorf("市场 %s 的代码 %q 无效", ex, code)
		}
	default:
		if len(code) != 6 || !isDigits(code) {
			return Instrument{}, fmt.Errorf("市场 %d 的代码 %q 须为六位数字", ex, code)
		}
	}
	return Instrument{Exchange: ex, Code: code}, nil
}

// isDigits 是否全部为数字
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isStockSymbol 是否美股代码(1~5位字母,可含 . / - 等特殊字符,如 BRK.B)
func isStockSymbol(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '.' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// IsStock 是否是股票,示例sz000001。
// 已注册市场先解析再按路由的标准协议与品种分类判断；公开支持的旧格式保留兜底。
func IsStock(code string) bool {
	if len(code) != 8 && !strings.ContainsAny(code, "./") {
		return false
	}
	if i, err := ParseInstrument(code); err == nil {
		if route, ok := i.Exchange.Route(); ok && route.Protocol == ProtocolStandard {
			if rules, ok := i.Rules(); ok {
				return rules.Security == SecurityStock
			}
		}
	}
	return IsSZStock(code) || IsSHStock(code) || IsBJStock(code)
}

// IsConvertibleBond reports whether code belongs to a current convertible-bond code range.
func IsConvertibleBond(code string) bool {
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	number := code[2:]
	switch code[:2] {
	case ExchangeSH.String():
		return isSHBond(number)
	case ExchangeSZ.String():
		return isSZBond(number)
	default:
		return false
	}
}

func IsSZStock(code string) bool {
	return len(code) == 8 && strings.ToLower(code[0:2]) == ExchangeSZ.String() && isSZStock(code[2:])
}

func IsSHStock(code string) bool {
	return len(code) == 8 && strings.ToLower(code[0:2]) == ExchangeSH.String() && isSHStock(code[2:])
}

func IsBJStock(code string) bool {
	return len(code) == 8 && strings.ToLower(code[0:2]) == ExchangeBJ.String() && isBJStock(code[2:])
}

func isSHStock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:1] == "6"
}

func isSZStock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:1] == "0" || code[:2] == "30"
}

func isBJStock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:2] == "92"
}

// IsETF 是否是基金,示例sz159558。
// 解析后仍按既有的 ETF/LOF 号段判断，避免把所有基金一概视为 ETF。
func IsETF(code string) bool {
	if len(code) != 8 && !strings.ContainsAny(code, "./") {
		return false
	}
	if i, err := ParseInstrument(code); err == nil {
		switch i.Exchange {
		case ExchangeSH:
			return isSHETF(i.Code)
		case ExchangeSZ:
			return isSZETF(i.Code)
		case ExchangeBJ:
			return isBJETF(i.Code)
		}
		return false
	}
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	switch {
	case code[0:2] == ExchangeSH.String() && isSHETF(code[2:]):
		return true
	case code[0:2] == ExchangeSZ.String() && isSZETF(code[2:]):
		return true
	}
	return false
}

func isSHETF(code string) bool {
	if len(code) != 6 {
		return false
	}
	switch code[:2] {
	case "50", "51", "52", "53", "56", "58": //55不是
		return true
	}

	return false
}

func isSZETF(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:2] == "15" || code[:2] == "16"
}

func isBJETF(code string) bool {
	if len(code) != 6 {
		return false
	}
	return false
}

// IsIndex 是否是指数,sh000001,sz399001,bj899100。
// 板块指数(880xxx 行业/概念, 881xxx 地域)归属上海交易所(ExchangeSH), 也视为指数。
// 带市场标识的标准代码（前缀、点后缀或完整市场键）统一解析后再分类；扩展市场不在公开支持范围内。
func IsIndex(code string) bool {
	if len(code) != 8 && !strings.ContainsAny(code, "./") {
		return false
	}
	if i, err := ParseInstrument(code); err == nil {
		switch i.Exchange {
		case ExchangeSH:
			return isSHIndex(i.Code) || isBlock(i.Code)
		case ExchangeSZ:
			return isSZIndex(i.Code)
		case ExchangeBJ:
			return isBJIndex(i.Code)
		}
		if route, ok := i.Exchange.Route(); ok && route.Protocol == ProtocolStandard {
			if rules, ok := i.Rules(); ok {
				return rules.Security == SecurityIndex
			}
		}
		return false
	}
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	switch {
	case code[0:2] == ExchangeSH.String() && isSHIndex(code[2:]):
		return true
	case code[0:2] == ExchangeSZ.String() && isSZIndex(code[2:]):
		return true
	case code[0:2] == ExchangeBJ.String() && isBJIndex(code[2:]):
		return true
	case code[0:2] == ExchangeSH.String() && isBlock(code[2:]):
		return true
	}
	return false
}

// isBlock 板块指数: 880xxx(概念/风格/地区) 881xxx(行业), 归属上海交易所。
func isBlock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "880" || code[:3] == "881"
}

func isSHIndex(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "000" || code == "999999"
}

func isSZIndex(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "399"
}

func isBJIndex(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "899"
}

// AddPrefix 添加股票/基金/指数/可转债代码前缀,针对股票/基金/指数/可转债生效,例如000001,会增加前缀sz000001(平安银行),而不是sh000001(上证指数)
// 板块指数(880xxx/881xxx)增加前缀 sh,例如 880741 -> sh880741(归属上海交易所)。
// 可转债: 沪市(110/111/113/118)增加前缀 sh, 深市(123/125/126/127/128)增加前缀 sz。
func AddPrefix(code string) string {
	if len(code) == 6 {
		switch {
		case isSHStock(code):
			return ExchangeSH.String() + code
		case isSZStock(code):
			return ExchangeSZ.String() + code
		case isBJStock(code):
			return ExchangeBJ.String() + code

		case isSHETF(code):
			return ExchangeSH.String() + code
		case isSZETF(code):
			return ExchangeSZ.String() + code
		case isBJETF(code):
			return ExchangeBJ.String() + code

		case isSHIndex(code):
			return ExchangeSH.String() + code
		case isSZIndex(code):
			return ExchangeSZ.String() + code
		case isBJIndex(code):
			return ExchangeBJ.String() + code

		case isBlock(code):
			return ExchangeSH.String() + code

		case isSHBond(code):
			return ExchangeSH.String() + code
		case isSZBond(code):
			return ExchangeSZ.String() + code
		}
	}
	return code
}

// isSHBond 沪市可转债: 110/111/113/118。
func isSHBond(code string) bool {
	if len(code) != 6 {
		return false
	}
	return strings.HasPrefix(code, "110") ||
		strings.HasPrefix(code, "111") ||
		strings.HasPrefix(code, "113") ||
		strings.HasPrefix(code, "118")
}

// isSZBond 深市可转债: 123/125/126/127/128。
func isSZBond(code string) bool {
	if len(code) != 6 {
		return false
	}
	return strings.HasPrefix(code, "123") ||
		strings.HasPrefix(code, "125") ||
		strings.HasPrefix(code, "126") ||
		strings.HasPrefix(code, "127") ||
		strings.HasPrefix(code, "128")
}
