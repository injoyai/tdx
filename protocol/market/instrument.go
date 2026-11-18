package market

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Instrument 标识一个行情品种；Exchange 是协议市场编号，Code 不含前缀。
type Instrument struct {
	Exchange Exchange
	Code     string
}

// String formats a security with its exchange abbreviation.
func (i Instrument) String() string {
	return i.Exchange.String() + "." + i.Code
}

// QueryCode formats an instrument so another query preserves its known wire market.
func (i Instrument) QueryCode() string {
	if UsesInstrumentSnapshot(i.Exchange) {
		return fmt.Sprintf("%s.%d", i.Code, i.Exchange.Uint8())
	}
	return i.String()
}

// Market returns the market associated with the instrument's wire exchange.
func (i Instrument) Market() (Market, error) {
	if m, ok := exchangeMarkets[i.Exchange]; ok {
		return m, nil
	}
	return "", fmt.Errorf("行情市场 %d 尚未接入统一 Client", i.Exchange)
}

// Kind reports the security type within its market.
func (i Instrument) Kind() SecurityKind {
	def := marketDefinitions[exchangeMarkets[i.Exchange]]
	for _, rule := range def.kinds {
		if rule.matches(i.Code) {
			return rule.kind
		}
	}
	if def.security != "" {
		return def.security
	}
	return SecurityOther
}

// Info returns the quote and K-line rules for the instrument.
func (i Instrument) Info() (MarketInfo, bool) {
	def, ok := marketDefinitions[exchangeMarkets[i.Exchange]]
	if !ok {
		return MarketInfo{}, false
	}
	if def.securityInfo != nil {
		info := def.securityInfo(i.Kind())
		info.TimeZone, info.CloseTime = def.info.TimeZone, def.info.CloseTime
		return info, true
	}
	return def.info, true
}

// StorageCode returns the code used with its market key in pull and storage.
func (i Instrument) StorageCode() string {
	if marketDefinitions[exchangeMarkets[i.Exchange]].info.Protocol == ProtocolStandard {
		return i.Exchange.String() + i.Code
	}
	return i.Code
}

type snapshotRule struct {
	code      func(string) (string, bool)
	exchanges []Exchange
}

// InstrumentLookup describes a code that needs a unique match in a snapshot.
type InstrumentLookup struct {
	Code      string
	Exchanges []Exchange
	prefix    string
}

func (l InstrumentLookup) String() string { return l.prefix + "." + l.Code }

// Candidates returns the candidate wire market numbers.
func (l InstrumentLookup) Candidates() string {
	numbers := make([]string, 0, len(l.Exchanges))
	for _, ex := range l.Exchanges {
		numbers = append(numbers, strconv.Itoa(int(ex)))
	}
	return strings.Join(numbers, "、")
}

// LookupInstrument finds the snapshot rule for an input, if any.
func LookupInstrument(input string) (InstrumentLookup, bool) {
	for _, m := range snapshotMarkets {
		def := marketDefinitions[m]
		if code, ok := def.lookup.code(input); ok {
			return InstrumentLookup{Code: code, Exchanges: slices.Clone(def.lookup.exchanges), prefix: def.prefix}, true
		}
	}
	return InstrumentLookup{}, false
}

// UsesInstrumentSnapshot reports whether an exchange participates in snapshot routing.
func UsesInstrumentSnapshot(ex Exchange) bool { return snapshotExchanges[ex] }

// ParseInstrument parses a bare code within an explicit market.
func (m Market) ParseInstrument(body string) (Instrument, error) {
	ex, ok := m.Exchange()
	if !ok {
		return Instrument{}, fmt.Errorf("未定义的行情市场 %s", m)
	}
	if parse := marketDefinitions[m].parseExplicit; parse != nil {
		if i, ok := parse(Instrument{ex, body}); ok && i.Exchange == ex {
			return i, nil
		}
	}
	i, err := checkedInstrument(ex, body)
	if err != nil {
		return Instrument{}, err
	}
	if i.Exchange != ex {
		return Instrument{}, fmt.Errorf("代码 %q 不属于市场 %s", body, m)
	}
	return i, nil
}

// ParseInstrument 支持拼接/点分隔前缀、交易所后缀和可判定的裸代码。
// 期货保留合约大小写；港股股票、基金和指数分别路由到市场 31、49 和 27。
func ParseInstrument(s string) (Instrument, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Instrument{}, fmt.Errorf("代码不能为空")
	}
	if ex, ok := bareInstruments[strings.ToUpper(s)]; ok {
		return checkedInstrument(ex, s)
	}
	// 原始协议的长前缀（如 cffo）优先，避免误拆为 cff + 合约主体。
	if ex, body, ok := splitPrefix(s); ok {
		if _, err := (Instrument{ex, body}).Market(); err != nil {
			return Instrument{}, err
		}
	}
	for _, ex := range instrumentExchanges {
		prefix := ex.String()
		if len(s) <= len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
			continue
		}
		body := s[len(prefix):]
		explicit := strings.HasPrefix(body, ".") || strings.HasPrefix(body, "/")
		if explicit {
			body = body[1:]
		}
		if marketDefinitions[exchangeMarkets[ex]].preferBareSymbol && !explicit && isStockSymbol(s) {
			continue // WIX 等裸美股代码保持原路由。
		}
		i, err := checkedInstrument(ex, body)
		if err == nil || explicit {
			return i, err
		}
	}
	if p := strings.LastIndexByte(s, '.'); p > 0 {
		suffix := s[p+1:]
		if ex, err := ParseExchange(suffix); err == nil {
			if m, ok := exchangeMarkets[ex]; ok && isDigits(suffix) {
				return m.ParseInstrument(s[:p])
			}
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
	m, err := i.Market()
	if err != nil {
		return Instrument{}, err
	}
	if i, ok := marketDefinitions[m].parse(i); ok {
		return i, nil
	}
	return Instrument{}, fmt.Errorf("市场 %s 的代码 %q 无效（标准代码6字节，扩展代码最多9字节）", ex, body)
}

func standardInstrumentCode(i Instrument) (Instrument, bool) {
	return i, len(i.Code) == 6 && isDigits(i.Code)
}

func numericHKCode(i Instrument) (Instrument, bool) {
	return i, len(i.Code) == 5 && isDigits(i.Code)
}

// symbolParts validates an extended wire code and describes its ASCII contents.
func symbolParts(code, separators string) (hasLetter, hasDigit, valid bool) {
	if len(code) == 0 || len(code) > 9 {
		return false, false, false
	}
	previousSeparator := true
	for _, c := range code {
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			hasLetter = true
		case !previousSeparator && strings.ContainsRune(separators, c):
			previousSeparator = true
			continue
		default:
			return false, false, false
		}
		previousSeparator = false
	}
	return hasLetter, hasDigit, !previousSeparator
}

func upperSymbol(i Instrument, separators string) (Instrument, bool) {
	letter, _, valid := symbolParts(i.Code, separators)
	i.Code = strings.ToUpper(i.Code)
	return i, valid && letter
}

func contractCode(i Instrument) (Instrument, bool) {
	letter, digit, valid := symbolParts(i.Code, "")
	return i, valid && letter && digit
}

func futuresIndexCode(i Instrument) (Instrument, bool) {
	letter, _, valid := symbolParts(i.Code, "")
	return i, valid && letter
}
