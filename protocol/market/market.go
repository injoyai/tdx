// Package market defines securities, exchange routing, schedules, and units.
package market

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// ID is a stable market key for configuration and storage.
type ID string

func (m ID) String() string { return string(m) }

// Market provides the rules consumed by the market registry and quote routing.
// Register snapshots Info. After registration Resolve and Rules may run concurrently
// and must preserve the registered routing and normalization contract.
type Market interface {
	Info() MarketInfo
	Resolve(code Code, snapshot InstrumentSnapshot) (Instrument, error)
	Rules(instrument Instrument) (InstrumentRules, error)
}

// instrumentImporter is an optional capability for markets that persist a
// discovered instrument assignment. Markets without local state need no import.
type instrumentImporter interface {
	Import(items []Instrument) error
}

// MarketInfo describes a logical market. Routes are in order of precedence,
// with the default route first. Schedule fields do not interpret source timestamps.
type MarketInfo struct {
	Prefix    string
	Name      string
	TimeZone  string
	CloseTime string
	Routes    []RouteInfo

	// Version identifies the Resolve and Rules contract of an application market.
	// Change it when either changes stored data or code assignment.
	Version string

	// PreferBareSymbol is part of the market definition fingerprint.
	// Dotless input is reserved for the built-in SH, SZ, BJ and HK markets.
	PreferBareSymbol bool
}

// RouteInfo describes one registered wire market and the corresponding storage key.
type RouteInfo struct {
	ID             ID
	Exchange       Exchange
	Name           string
	Protocol       MarketProtocol
	CodeSource     CodeSource
	DiscoverBlocks bool
	// BareSymbols lists the built-in route symbols accepted without a prefix.
	BareSymbols []string
	Snapshot    bool
	// Report selects the archive member and parser for report-backed code lists.
	// It must be set when CodeSource is CodeListZHBReport.
	Report *CodeReport
	// QuoteListCategory declares the batch quote list layout (2=HK, 3=futures).
	// Zero means the route has no batch quote list.
	QuoteListCategory uint8
	// Session is the clock for layouts that derive minute timestamps from
	// record order. Zero means such layouts are not supported by this route.
	Session TradingSession
}

// ReportCode identifies a parser bound by the layer that owns the decoder.
type ReportCode string

const (
	ReportTdxBjMore ReportCode = "tdxbjmore"
)

// CodeReport declares how a report-backed market obtains its code list.
type CodeReport struct {
	Member string     // 压缩包成员名
	Parser ReportCode // 解码层绑定的解析器标识
}

type CodeSource uint8

const (
	CodeListStandard CodeSource = iota
	CodeListExtended
	CodeListZHBReport
)

// Code is the symbol and route selected by the input parser. An explicit wire
// market is validated within that route. Shared identifiers use a snapshot.
type Code struct {
	Symbol   string
	Exchange Exchange
	explicit bool
}

// Explicit reports whether the input selected an exact wire route.
func (c Code) Explicit() bool { return c.explicit }

func validateInstrument(ex Exchange, code Code, valid bool) (Instrument, error) {
	if err := checkExchange(code.Exchange, ex); err != nil {
		return Instrument{}, err
	}
	if !valid {
		return Instrument{}, fmt.Errorf("市场 %s 的代码 %q 无效（标准代码6字节，扩展代码最多9字节）", ex, code.Symbol)
	}
	return Instrument{Exchange: ex, Code: code.Symbol}, nil
}

func checkExchange(got, want Exchange) error {
	if got != want {
		return fmt.Errorf("行情市场 %d 不属于 %s", got, want)
	}
	return nil
}

type registeredRoute struct {
	RouteInfo
	owner *registeredMarket
}

type registeredMarket struct {
	Market
	info     MarketInfo
	identity string
}

var (
	marketKey           = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	marketMu            sync.RWMutex
	prefixMarkets       = map[string]*registeredMarket{}
	marketRoutes        = map[ID]RouteInfo{}
	exchangeRoutes      = map[Exchange]registeredRoute{}
	markets             []ID
	instrumentExchanges []Exchange
	bareInstruments     = map[string]Exchange{}
)

// registerMarket registers one logical market and indexes its wire routes.
func registerMarket(market Market) {
	if err := register(market, false); err != nil {
		panic(err)
	}
}

// Register adds one market object and all its wire routes. A failed registration
// leaves every index unchanged.
func Register(m Market) error { return register(m, true) }

func register(m Market, external bool) error {
	if m == nil || (reflect.ValueOf(m).Kind() == reflect.Pointer && reflect.ValueOf(m).IsNil()) {
		return fmt.Errorf("市场对象不能为空")
	}
	info := cloneInfo(m.Info())
	if (info.TimeZone == "") != (info.CloseTime == "") {
		return fmt.Errorf("市场 %s 的时区和收盘时间须同时提供", info.Prefix)
	}
	if info.TimeZone != "" {
		if clock, err := time.Parse("15:04", info.CloseTime); err != nil || clock.Format("15:04") != info.CloseTime {
			return fmt.Errorf("市场 %s 的收盘时间 %q 无效", info.Prefix, info.CloseTime)
		}
	}
	entry := &registeredMarket{Market: m, info: info}
	if configured, ok := m.(customMarket); ok {
		entry.identity = configured.identity()
	} else if external {
		// Schedule changes affect the update window, not the stored data definition.
		data, _ := json.Marshal(struct {
			Prefix, Version  string
			PreferBareSymbol bool
			Routes           []RouteInfo
		}{info.Prefix, info.Version, info.PreferBareSymbol, info.Routes})
		entry.identity = string(data)
	}

	marketMu.Lock()
	defer marketMu.Unlock()
	if !validMarketKey(info.Prefix) || prefixMarkets[info.Prefix] != nil ||
		marketRoutes[ID(info.Prefix)].ID != "" || len(info.Routes) == 0 {
		return fmt.Errorf("无效或重复的市场简称 %q", info.Prefix)
	}
	if err := checkMarketNames(info); err != nil {
		return err
	}
	ids, exchanges, symbols := map[ID]bool{}, map[Exchange]bool{}, map[string]bool{}
	for _, route := range info.Routes {
		if _, exists := marketRoutes[route.ID]; exists || ids[route.ID] {
			return fmt.Errorf("重复定义市场 %s", route.ID)
		}
		if _, exists := exchangeRoutes[route.Exchange]; exists || exchanges[route.Exchange] {
			return fmt.Errorf("重复定义协议市场 %d", route.Exchange)
		}
		if !validRouteID(route.ID) || prefixMarkets[route.ID.String()] != nil ||
			(route.Protocol != ProtocolStandard && route.Protocol != ProtocolExtended) {
			return fmt.Errorf("无效市场定义 %q", route.ID)
		}
		if (route.Protocol == ProtocolStandard && route.CodeSource != CodeListStandard && route.CodeSource != CodeListZHBReport) ||
			(route.Protocol == ProtocolExtended && route.CodeSource != CodeListExtended) {
			return fmt.Errorf("市场 %s 的代码来源 %d 与协议 %s 不匹配", route.ID, route.CodeSource, route.Protocol)
		}
		if route.QuoteListCategory != 0 && (route.Protocol != ProtocolExtended || (route.QuoteListCategory != 2 && route.QuoteListCategory != 3)) {
			return fmt.Errorf("市场 %s 的批量报价布局 %d 无效", route.ID, route.QuoteListCategory)
		}
		if route.Session != (TradingSession{}) && !route.Session.Valid() {
			return fmt.Errorf("市场 %s 的交易时段无效", route.ID)
		}
		if route.CodeSource == CodeListZHBReport &&
			(route.Report == nil || route.Report.Member == "" || route.Report.Parser == "") {
			return fmt.Errorf("市场 %s 的报表来源缺少成员或解析器", route.ID)
		}
		for _, symbol := range route.BareSymbols {
			symbol = strings.ToUpper(symbol)
			if symbol == "" || symbol != strings.TrimSpace(symbol) {
				return fmt.Errorf("无效裸代码 %q", symbol)
			}
			if !compactExchange(route.Exchange) {
				continue
			}
			if _, exists := bareInstruments[symbol]; exists || symbols[symbol] {
				return fmt.Errorf("重复定义裸代码 %q", symbol)
			}
			symbols[symbol] = true
		}
		ids[route.ID], exchanges[route.Exchange] = true, true
	}
	prefixMarkets[info.Prefix] = entry
	for _, route := range info.Routes {
		for _, symbol := range route.BareSymbols {
			if compactExchange(route.Exchange) {
				bareInstruments[strings.ToUpper(symbol)] = route.Exchange
			}
		}
		marketRoutes[route.ID] = route
		exchangeRoutes[route.Exchange] = registeredRoute{route, entry}
		markets = append(markets, route.ID)
	}
	slices.Sort(markets)
	instrumentExchanges = append(instrumentExchanges, info.Routes[0].Exchange)
	slices.SortFunc(instrumentExchanges, func(a, b Exchange) int {
		ap, bp := exchangeRoutes[a].owner.info.Prefix, exchangeRoutes[b].owner.info.Prefix
		if d := len(bp) - len(ap); d != 0 {
			return d
		}
		return strings.Compare(ap, bp)
	})
	return nil
}

func validMarketKey(key string) bool {
	return key != "hi" && key != "unknown" && marketKey.MatchString(key)
}

func validRouteID(id ID) bool {
	for _, part := range strings.Split(id.String(), "/") {
		if !validMarketKey(part) {
			return false
		}
	}
	return true
}

// checkMarketNames keeps shorthand and raw protocol aliases unambiguous.
func checkMarketNames(info MarketInfo) error {
	for _, ex := range rawExchanges {
		prefix, name := ex.rawPrefix(), ex.rawName()
		if strings.EqualFold(info.Prefix, name) || (info.Prefix == prefix && info.Routes[0].Exchange != ex) {
			return fmt.Errorf("市场简称 %q 已用于协议市场 %d", info.Prefix, ex)
		}
		for _, route := range info.Routes {
			if (ex == route.Exchange && info.Prefix != prefix) ||
				(ex != route.Exchange && (route.ID.String() == prefix || strings.EqualFold(route.Name, name) || strings.EqualFold(route.Name, prefix))) {
				return fmt.Errorf("市场 %s 与协议市场 %d 的既有名称冲突", route.ID, ex)
			}
		}
	}
	for _, existing := range exchangeRoutes {
		if strings.EqualFold(info.Prefix, existing.Name) {
			return fmt.Errorf("市场简称 %q 已用于协议市场 %d", info.Prefix, existing.Exchange)
		}
		for _, route := range info.Routes {
			if route.Name != "" && (strings.EqualFold(route.Name, existing.Name) ||
				strings.EqualFold(route.Name, existing.owner.info.Prefix) || strings.EqualFold(route.Name, existing.ID.String())) {
				return fmt.Errorf("市场名称 %q 已用于协议市场 %d", route.Name, existing.Exchange)
			}
		}
	}
	return nil
}

func cloneInfo(info MarketInfo) MarketInfo {
	info.Routes = slices.Clone(info.Routes)
	for i := range info.Routes {
		info.Routes[i] = cloneRoute(info.Routes[i])
	}
	return info
}

func cloneRoute(info RouteInfo) RouteInfo {
	if info.Report != nil {
		report := *info.Report
		info.Report = &report
	}
	info.BareSymbols = slices.Clone(info.BareSymbols)
	return info
}

func routeFor(ex Exchange) (registeredRoute, bool) {
	marketMu.RLock()
	defer marketMu.RUnlock()
	route, ok := exchangeRoutes[ex]
	return route, ok
}

// Identity returns the persistent data definition of an application market.
// Built-in markets retain their existing data namespace.
func (m ID) Identity() string {
	if ex, ok := m.Exchange(); ok {
		route, _ := routeFor(ex)
		return route.owner.identity
	}
	return ""
}

// Markets returns the registered configuration and storage keys in lexical order.
func Markets() []ID {
	marketMu.RLock()
	defer marketMu.RUnlock()
	return slices.Clone(markets)
}

// StandardExchanges returns exchanges supported by the standard wire protocol.
func StandardExchanges() []Exchange {
	marketMu.RLock()
	defer marketMu.RUnlock()
	var out []Exchange
	for _, id := range markets {
		if route := marketRoutes[id]; route.Protocol == ProtocolStandard {
			out = append(out, route.Exchange)
		}
	}
	slices.Sort(out)
	return out
}

// Exchange returns the wire identifier associated with this storage key.
func (m ID) Exchange() (Exchange, bool) {
	marketMu.RLock()
	defer marketMu.RUnlock()
	route, ok := marketRoutes[m]
	return route.Exchange, ok
}

// DiscoversBlocks reports whether this market includes sector indices from block_zs.dat.
func (m ID) DiscoversBlocks() bool {
	marketMu.RLock()
	defer marketMu.RUnlock()
	return marketRoutes[m].DiscoverBlocks
}

// Info returns the logical market and all of its protocol routes.
func (m ID) Info() (MarketInfo, bool) {
	if ex, ok := m.Exchange(); ok {
		route, _ := routeFor(ex)
		return cloneInfo(route.owner.info), true
	}
	return MarketInfo{}, false
}

// Route returns the protocol metadata associated with the wire exchange.
// 返回值与注册表隔离，报表配置与裸代码列表均为独立副本。
func (e Exchange) Route() (RouteInfo, bool) {
	route, ok := routeFor(e)
	if !ok {
		return RouteInfo{}, false
	}
	return cloneRoute(route.RouteInfo), true
}
