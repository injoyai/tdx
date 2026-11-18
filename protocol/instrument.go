package protocol

import (
	"fmt"
	"strings"
)

// Instrument 标识一个行情品种；Exchange 是协议市场编号，Code 不含前缀。
type Instrument struct {
	Exchange Exchange
	Code     string
}

func (i Instrument) String() string {
	if i.Exchange == ExchangeHI {
		return "hk." + i.Code
	}
	return i.Exchange.String() + "." + i.Code
}

// Market 返回已接入行情的市场类别。
func (i Instrument) Market() (Market, error) {
	switch i.Exchange {
	case ExchangeSZ, ExchangeSH, ExchangeBJ:
		code := i.Exchange.String() + i.Code
		switch {
		case IsETF(code):
			return MarketEtfLof, nil
		case i.Exchange == ExchangeSH && isBlock(i.Code):
			return MarketBlock, nil
		case IsIndex(code):
			return MarketIndex, nil
		case IsConvertibleBond(code):
			return MarketBond, nil
		case IsStock(code):
			return MarketAStock, nil
		default:
			return MarketOther, nil
		}
	case ExchangeHK:
		return MarketHK, nil
	case ExchangeHKFund:
		return MarketHKFund, nil
	case ExchangeHI:
		return MarketHKIndex, nil
	case ExchangeUS:
		return MarketUS, nil
	case ExchangeCFF, ExchangeCZC, ExchangeDCE, ExchangeSHF, ExchangeGFE, ExchangeQHZ:
		return MarketFuture, nil
	default:
		return "", fmt.Errorf("行情市场 %d 尚未接入统一 Client", i.Exchange)
	}
}

// ParseInstrument 支持拼接/点分隔前缀、交易所后缀和可判定的裸代码。
// 期货保留合约大小写；港股股票、基金和指数分别路由到市场 31、49 和 27。
func ParseInstrument(s string) (Instrument, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Instrument{}, fmt.Errorf("代码不能为空")
	}
	switch strings.ToUpper(s) {
	case "HSI", "HKL", "VHSI", "CES100", "GEM":
		return Instrument{ExchangeHI, strings.ToUpper(s)}, nil
	}
	// 原始协议的长前缀（如 cffo）优先，避免误拆为 cff + 合约主体。
	if ex, body, ok := splitPrefix(s); ok {
		if _, err := (Instrument{ex, body}).Market(); err != nil {
			return Instrument{}, err
		}
	}
	for _, ex := range []Exchange{ExchangeCFF, ExchangeCZC, ExchangeDCE, ExchangeSHF, ExchangeGFE, ExchangeQHZ, ExchangeSZ, ExchangeSH, ExchangeBJ, ExchangeHKFund, ExchangeHK, ExchangeHI, ExchangeUS} {
		prefix := ex.String()
		if len(s) <= len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
			continue
		}
		body := s[len(prefix):]
		explicit := strings.HasPrefix(body, ".") || strings.HasPrefix(body, "/")
		if explicit {
			body = body[1:]
		}
		i, err := checkedInstrument(ex, body)
		if err == nil || explicit {
			return i, err
		}
	}
	if p := strings.LastIndexByte(s, '.'); p > 0 {
		if ex, err := ParseExchange(s[p+1:]); err == nil {
			return checkedInstrument(ex, s[:p])
		}
	}
	if len(s) == 6 && isDigits(s) && strings.HasPrefix(s, "89") {
		return checkedInstrument(ExchangeBJ, s)
	}
	ex, body, err := decodeCode(s)
	if err != nil {
		return Instrument{}, err
	}
	return checkedInstrument(ex, body)
}

func checkedInstrument(ex Exchange, body string) (Instrument, error) {
	i := Instrument{ex, body}
	if _, err := i.Market(); err != nil {
		return Instrument{}, err
	}
	valid := len(body) > 0 && len(body) <= 9
	hasLetter, hasDigit := false, false
	for _, c := range body {
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			hasLetter = true
		case ex == ExchangeUS && (c == '.' || c == '-'):
		default:
			valid = false
		}
	}
	switch ex {
	case ExchangeSZ, ExchangeSH, ExchangeBJ:
		valid = len(body) == 6 && isDigits(body)
	case ExchangeHK:
		if hasLetter {
			i.Exchange = ExchangeHI
		} else {
			valid = valid && len(body) == 5 && isDigits(body)
			if valid && isHKETFCode(body) {
				i.Exchange = ExchangeHKFund
			}
		}
		i.Code = strings.ToUpper(body)
	case ExchangeHKFund:
		valid = len(body) == 5 && isDigits(body)
	case ExchangeHI:
		valid = valid && hasLetter
		i.Code = strings.ToUpper(body)
	case ExchangeUS:
		valid = valid && hasLetter
		for _, part := range strings.Split(strings.ReplaceAll(body, ".", "-"), "-") {
			valid = valid && part != ""
		}
		i.Code = strings.ToUpper(body)
	default:
		valid = valid && hasLetter && hasDigit
	}
	if !valid {
		return Instrument{}, fmt.Errorf("市场 %s 的代码 %q 无效（标准代码6字节，扩展代码最多9字节）", ex, body)
	}
	return i, nil
}

// isHKETFCode 依据港交所证券代号编配计划匹配五位数字 ETF 代码。
// 市场 49 的其他基金品种可用 hkf 显式指定。
// https://www.hkex.com.hk/Products/Securities/Stock-Code-Allocation-Plan
func isHKETFCode(code string) bool {
	for _, span := range [][2]string{
		{"02800", "02849"}, {"03000", "03199"}, {"03400", "03499"}, // 港元
		{"09000", "09199"}, {"09400", "09499"}, {"09800", "09849"}, // 美元
		{"82800", "82849"}, {"83000", "83199"}, {"83400", "83499"}, // 人民币
	} {
		if code >= span[0] && code <= span[1] {
			return true
		}
	}
	return false
}
