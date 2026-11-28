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
	route, ok := routeFor(i.Exchange)
	if ok {
		info := route.owner.info
		if route.Snapshot || (len(info.Routes) > 1 && route.ID.String() == info.Prefix) {
			return fmt.Sprintf("%s.%d", i.Code, i.Exchange.Uint8())
		}
		if len(info.Routes) > 1 {
			return route.ID.String() + "." + i.Code
		}
	}
	return i.String()
}

// CodeKey preserves the compact code used by single-route standard markets
// and uses an exact route for securities sharing a market prefix.
func (i Instrument) CodeKey() string {
	if route, ok := routeFor(i.Exchange); ok && route.Protocol == ProtocolStandard && len(route.owner.info.Routes) == 1 {
		return route.owner.info.Prefix + i.Code
	}
	return i.QueryCode()
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
// 协议类型以路由声明为准，规则实现无需重复声明。
func (i Instrument) Rules() (InstrumentRules, bool) {
	route, ok := routeFor(i.Exchange)
	if !ok {
		return InstrumentRules{}, false
	}
	rules, err := route.owner.Rules(i)
	if err != nil {
		return InstrumentRules{}, false
	}
	rules.Protocol = route.Protocol
	return rules, true
}

// StorageCode returns the code used with its market key in pull and storage.
func (i Instrument) StorageCode() string {
	route, _ := routeFor(i.Exchange)
	if route.Protocol == ProtocolStandard {
		return i.Exchange.String() + i.Code
	}
	return i.Code
}

// InstrumentLookup describes a code that needs a unique match in a snapshot.
type InstrumentLookup struct {
	Code          string
	Exchanges     []Exchange
	Prefix        string
	knownExchange Exchange
}

func (l InstrumentLookup) String() string { return l.Prefix + "." + l.Code }

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

// ResolveInstrument asks the market to resolve the code. A nil loader uses the
// market's imported and compiled assignments for codes that would otherwise need a snapshot.
func ResolveInstrument(input string, load func(InstrumentLookup) (InstrumentSnapshot, error)) (Instrument, error) {
	i, err := ParseInstrument(input)
	var required *snapshotRequired
	if !errors.As(err, &required) {
		return i, err
	}
	lookup := required.lookup
	var snapshot InstrumentSnapshot
	if load != nil {
		var loadErr error
		snapshot, loadErr = load(lookup)
		if loadErr != nil {
			return Instrument{}, fmt.Errorf("%s 无法读取通达信品种快照（候选市场 %s）: %w", lookup, lookup.Candidates(), loadErr)
		}
	}
	// A supplied snapshot takes precedence over the market's known assignments.
	if !snapshot.Loaded() && lookup.knownExchange != 0 {
		return Instrument{Exchange: lookup.knownExchange, Code: lookup.Code}, nil
	}
	route, ok := routeFor(lookup.Exchanges[0])
	if !ok {
		return Instrument{}, fmt.Errorf("未定义的快照候选市场 %d", lookup.Exchanges[0])
	}
	return route.resolve(Code{Symbol: lookup.Code, Exchange: lookup.Exchanges[0]}, snapshot)
}

// UsesInstrumentSnapshot reports whether an exchange participates in snapshot routing.
func UsesInstrumentSnapshot(ex Exchange) bool {
	route, _ := routeFor(ex)
	return route.Snapshot
}

// ParseInstrument parses a bare code within an explicit market.
func (m ID) ParseInstrument(body string) (Instrument, error) {
	ex, ok := m.Exchange()
	if !ok {
		return Instrument{}, fmt.Errorf("未定义的行情市场 %s", m)
	}
	route, _ := routeFor(ex)
	return route.resolve(Code{Symbol: body, Exchange: ex, explicit: true}, InstrumentSnapshot{})
}

// ParseInstrument 支持完整市场键、显式市场前缀/后缀和可判定的裸代码。
// 解析顺序固定为：完整市场键 → 显式前缀 → 显式后缀 → 无分隔前缀与裸代码，
// 避免代码主体中的点（如 WIX.US）被更短的市场前缀抢占。
// 期货保留合约大小写；港股股票、基金和指数分别路由到市场 31、49 和 27。
func ParseInstrument(s string) (Instrument, error) {
	resolve := func(ex Exchange, body string) (Instrument, error) {
		route, ok := routeFor(ex)
		if !ok {
			return Instrument{}, fmt.Errorf("行情市场 %d 尚未接入统一 Client", ex)
		}
		return route.resolve(Code{Symbol: body, Exchange: ex}, InstrumentSnapshot{})
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return Instrument{}, fmt.Errorf("代码不能为空")
	}
	marketMu.RLock()
	ex, bare := bareInstruments[strings.ToUpper(s)]
	marketMu.RUnlock()
	if bare {
		return resolve(ex, s)
	}
	// 1. 完整市场键显式选择协议路由，代码主体可以包含点。
	if prefix, body, ok := strings.Cut(s, "."); ok {
		if id, exact := exactRoute(prefix); exact {
			return id.ParseInstrument(body)
		}
	}
	if p := strings.LastIndexByte(s, '.'); p > 0 && strings.Contains(s[p+1:], "/") {
		id := ID(strings.ToLower(s[p+1:]))
		if _, registered := id.Exchange(); registered {
			return id.ParseInstrument(s[:p])
		}
	}
	// 2. 显式前缀：前缀后的第一个点或斜杠只是分隔符，其余部分属于代码主体。
	//    显式前缀再早于点后缀处理，保证 "WIX.US" 不会被 "wi" 前缀拆成 X.US。
	if i := strings.IndexAny(s, "./"); i > 0 {
		if ex, ok := parseMarketToken(s[:i]); ok {
			body := s[i+1:]
			if body != "" {
				route, _ := routeFor(ex)
				if strings.EqualFold(s[:i], route.Name) && route.Name != route.owner.info.Name {
					return route.ID.ParseInstrument(body)
				}
				return resolve(ex, body)
			}
		}
	}
	// 3. 显式后缀：数字编号必须落在 0-255，越界立即报错，不能回绕到其他市场。
	if p := strings.LastIndexByte(s, '.'); p > 0 {
		suffix := s[p+1:]
		if id, exact := exactRoute(suffix); exact {
			return id.ParseInstrument(s[:p])
		}
		if isDigits(suffix) {
			n, err := strconv.Atoi(suffix)
			if err != nil || n > 255 {
				return Instrument{}, fmt.Errorf("市场编号 %q 超出协议编号范围 0-255", suffix)
			}
			ex := Exchange(n)
			if route, ok := routeFor(ex); ok {
				return route.ID.ParseInstrument(s[:p])
			}
			return resolve(ex, s[:p])
		}
		if ex, err := ParseExchange(suffix); err == nil {
			if route, ok := routeFor(ex); ok &&
				(strings.EqualFold(suffix, route.Name) && route.Name != route.owner.info.Name) {
				return route.ID.ParseInstrument(s[:p])
			}
			return resolve(ex, s[:p])
		}
	}
	// 4. 无分隔前缀（如 sz000001）与裸代码推断。
	// 原始协议的长前缀（如 cffo）优先，避免误拆为 cff + 合约主体。
	if ex, body, ok := splitPrefix(s); ok {
		if _, err := (Instrument{ex, body}).Market(); err != nil {
			return Instrument{}, err
		}
		route, _ := routeFor(ex)
		if route.Name != "" && strings.HasPrefix(strings.ToLower(s), strings.ToLower(route.Name)) && route.Name != route.owner.info.Name {
			return route.ID.ParseInstrument(body)
		}
	}
	for _, ex := range instrumentRoutes() {
		prefix := ex.String()
		if len(s) <= len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
			continue
		}
		body := s[len(prefix):]
		explicit := strings.HasPrefix(body, ".") || strings.HasPrefix(body, "/")
		if explicit {
			body = body[1:]
		}
		route, _ := routeFor(ex)
		if route.owner.info.PreferBareSymbol && !explicit && isStockSymbol(s) {
			continue // WIX 等裸美股代码保持原路由。
		}
		i, err := resolve(ex, body)
		if err == nil || explicit {
			return i, err
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

// exactRoute recognizes a route key distinct from the logical market prefix.
func exactRoute(token string) (ID, bool) {
	id := ID(strings.ToLower(token))
	ex, ok := id.Exchange()
	if !ok {
		return "", false
	}
	route, _ := routeFor(ex)
	return id, route.owner.info.Prefix != id.String()
}

func (r registeredRoute) resolve(code Code, snapshot InstrumentSnapshot) (Instrument, error) {
	i, err := r.owner.Resolve(code, snapshot)
	if err != nil {
		return Instrument{}, err
	}
	target, ok := routeFor(i.Exchange)
	if !ok || target.owner != r.owner || (code.Explicit() && i.Exchange != r.Exchange) {
		return Instrument{}, fmt.Errorf("代码 %q 解析到了不匹配的协议市场 %d", code.Symbol, i.Exchange)
	}
	return i, nil
}

// parseMarketToken 解析显式前缀里市场标识部分（不含斜杠的简称、缩写或中文名）。
func parseMarketToken(token string) (Exchange, bool) {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" || strings.Contains(token, "/") {
		return 0, false
	}
	marketMu.RLock()
	m, known := prefixMarkets[token]
	marketMu.RUnlock()
	if known {
		return m.info.Routes[0].Exchange, true
	}
	for _, e := range allExchanges() {
		if token == strings.ToLower(e.String()) || token == strings.ToLower(e.Name()) {
			return e, true
		}
	}
	return 0, false
}

func checkedInstrument(ex Exchange, body string) (Instrument, error) {
	route, ok := routeFor(ex)
	if !ok {
		return Instrument{}, fmt.Errorf("行情市场 %d 尚未接入统一 Client", ex)
	}
	return route.resolve(Code{Symbol: body, Exchange: ex}, InstrumentSnapshot{})
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
