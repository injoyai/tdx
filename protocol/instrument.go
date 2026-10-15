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

// Market 返回已接入行情的市场类别；不据此推断扩展市场的股票/ETF/指数类型。
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
		default:
			return MarketAStock, nil
		}
	case ExchangeHK:
		return MarketHK, nil
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
// 期货保留合约大小写；港股股票和指数分别路由到市场 31 和 27。
func ParseInstrument(s string) (Instrument, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Instrument{}, fmt.Errorf("代码不能为空")
	}
	switch strings.ToUpper(s) {
	case "HSI", "HKL", "VHSI", "CES100", "GEM":
		return Instrument{ExchangeHI, strings.ToUpper(s)}, nil
	}
	for _, ex := range []Exchange{ExchangeCFF, ExchangeCZC, ExchangeDCE, ExchangeSHF, ExchangeGFE, ExchangeQHZ, ExchangeSZ, ExchangeSH, ExchangeBJ, ExchangeHK, ExchangeHI, ExchangeUS} {
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
	ex, body, err := DecodeCode(s)
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
		}
		i.Code = strings.ToUpper(body)
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
