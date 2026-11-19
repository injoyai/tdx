package market

import (
	"errors"
	"fmt"
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

// Market returns the storage key associated with the instrument's wire exchange.
func (i Instrument) Market() (ID, error) {
	if route, ok := i.Exchange.Route(); ok {
		return route.ID, nil
	}
	return "", fmt.Errorf("行情市场 %d 尚未接入统一 Client", i.Exchange)
}

// Kind reports the security type within its market.
func (i Instrument) Kind() SecurityKind {
	if rules, ok := i.Rules(); ok {
		return rules.Security
	}
	return SecurityOther
}

// Rules returns the quote, K-line, time-share, and trade rules for the instrument.
func (i Instrument) Rules() (InstrumentRules, bool) {
	if route, ok := exchangeRoutes[i.Exchange]; ok {
		rules, err := route.market.Rules(i)
		return rules, err == nil
	}
	return InstrumentRules{}, false
}

// StorageCode returns the code used with its market key in pull and storage.
func (i Instrument) StorageCode() string {
	if exchangeRoutes[i.Exchange].Protocol == ProtocolStandard {
		return i.Exchange.String() + i.Code
	}
	return i.Code
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
	_, err := ParseInstrument(input)
	var required *snapshotRequired
	if errors.As(err, &required) {
		return required.lookup, true
	}
	return InstrumentLookup{}, false
}

// ResolveInstrument asks the market to resolve the code, loading a snapshot
// only when its Resolve method reports the missing dependency.
func ResolveInstrument(input string, load func(InstrumentLookup) (InstrumentSnapshot, error)) (Instrument, error) {
	i, err := ParseInstrument(input)
	var required *snapshotRequired
	if !errors.As(err, &required) || load == nil {
		return i, err
	}
	lookup := required.lookup
	snapshot, err := load(lookup)
	if err != nil {
		return Instrument{}, fmt.Errorf("%s 无法读取通达信品种快照（候选市场 %s）: %w", lookup, lookup.Candidates(), err)
	}
	route, ok := exchangeRoutes[lookup.Exchanges[0]]
	if !ok {
		return Instrument{}, fmt.Errorf("未定义的快照候选市场 %d", lookup.Exchanges[0])
	}
	return route.market.Resolve(Code{Symbol: lookup.Code, Exchange: lookup.Exchanges[0]}, snapshot)
}

// UsesInstrumentSnapshot reports whether an exchange participates in snapshot routing.
func UsesInstrumentSnapshot(ex Exchange) bool { return exchangeRoutes[ex].Snapshot }

// ParseInstrument parses a bare code within an explicit market.
func (m ID) ParseInstrument(body string) (Instrument, error) {
	ex, ok := m.Exchange()
	if !ok {
		return Instrument{}, fmt.Errorf("未定义的行情市场 %s", m)
	}
	return exchangeRoutes[ex].market.Resolve(Code{Symbol: body, Exchange: ex, explicit: true}, InstrumentSnapshot{})
}

// ParseInstrument 支持拼接/点分隔前缀、交易所后缀和可判定的裸代码。
// 期货保留合约大小写；港股股票、基金和指数分别路由到市场 31、49 和 27。
func ParseInstrument(s string) (Instrument, error) {
	resolve := func(ex Exchange, body string) (Instrument, error) {
		route, ok := exchangeRoutes[ex]
		if !ok {
			return Instrument{}, fmt.Errorf("行情市场 %d 尚未接入统一 Client", ex)
		}
		return route.market.Resolve(Code{Symbol: body, Exchange: ex}, InstrumentSnapshot{})
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return Instrument{}, fmt.Errorf("代码不能为空")
	}
	if ex, ok := bareInstruments[strings.ToUpper(s)]; ok {
		return resolve(ex, s)
	}
	// 原始协议的长前缀（如 cffo）优先，避免误拆为 cff + 合约主体。
	if ex, body, ok := splitPrefix(s); ok {
		if _, err := (Instrument{ex, body}).Market(); err != nil {
			return Instrument{}, err
		}
		route := exchangeRoutes[ex]
		if strings.HasPrefix(s, route.Name) && route.Name != route.market.Info().Name {
			return route.ID.ParseInstrument(body)
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
		if exchangeRoutes[ex].preferBareSymbol && !explicit && isStockSymbol(s) {
			continue // WIX 等裸美股代码保持原路由。
		}
		i, err := resolve(ex, body)
		if err == nil || explicit {
			return i, err
		}
	}
	if p := strings.LastIndexByte(s, '.'); p > 0 {
		suffix := s[p+1:]
		if ex, err := ParseExchange(suffix); err == nil {
			if route, ok := exchangeRoutes[ex]; ok &&
				(isDigits(suffix) || (suffix == route.Name && route.Name != route.market.Info().Name)) {
				return route.ID.ParseInstrument(s[:p])
			}
			return resolve(ex, s[:p])
		}
	}
	if len(s) == 6 && isDigits(s) && strings.HasPrefix(s, "89") {
		return resolve(ExchangeBJ, s)
	}
	ex, body, err := decodeCode(s)
	if err != nil {
		return Instrument{}, err
	}
	return resolve(ex, body)
}

func checkedInstrument(ex Exchange, body string) (Instrument, error) {
	route, ok := exchangeRoutes[ex]
	if !ok {
		return Instrument{}, fmt.Errorf("行情市场 %d 尚未接入统一 Client", ex)
	}
	return route.market.Resolve(Code{Symbol: body, Exchange: ex}, InstrumentSnapshot{})
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
