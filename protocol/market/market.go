// Package market defines securities, exchange routing, schedules, and units.
package market

import (
	"fmt"
	"slices"
	"strings"
)

// ID is a stable market key for configuration and storage.
type ID string

func (m ID) String() string { return string(m) }

// Market provides the rules consumed by the market registry and quote routing.
type Market interface {
	Info() MarketInfo
	Resolve(code Code, snapshot InstrumentSnapshot) (Instrument, error)
	Rules(instrument Instrument) (InstrumentRules, error)
	// Import receives a completed instrument discovery result.
	Import(items []Instrument) error
}

type ignoreImport struct{}

func (ignoreImport) Import([]Instrument) error { return nil }

// MarketInfo describes a logical market. Routes are in order of precedence,
// with the default route first. Schedule fields do not interpret source timestamps.
type MarketInfo struct {
	Prefix    string
	Name      string
	TimeZone  string
	CloseTime string
	Routes    []RouteInfo

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
	BareSymbols    []string
	Snapshot       bool
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
	market           Market
	prefix           string
	preferBareSymbol bool
}

var (
	prefixMarkets       = map[string]Market{}
	marketRoutes        = map[ID]RouteInfo{}
	exchangeRoutes      = map[Exchange]registeredRoute{}
	markets             []ID
	instrumentExchanges []Exchange
	bareInstruments     = map[string]Exchange{}
)

// registerMarket registers one logical market and indexes its wire routes.
func registerMarket(market Market) {
	info := market.Info()
	if info.Prefix == "" || info.Prefix != strings.ToLower(info.Prefix) || prefixMarkets[info.Prefix] != nil || len(info.Routes) == 0 {
		panic(fmt.Sprintf("无效或重复的市场简称 %q", info.Prefix))
	}
	prefixMarkets[info.Prefix] = market
	for _, route := range info.Routes {
		if _, exists := marketRoutes[route.ID]; exists {
			panic(fmt.Sprintf("重复定义市场 %s", route.ID))
		}
		if _, exists := exchangeRoutes[route.Exchange]; exists {
			panic(fmt.Sprintf("重复定义协议市场 %d", route.Exchange))
		}
		if route.ID == "" || route.ID.String() != strings.ToLower(route.ID.String()) ||
			(route.Protocol != ProtocolStandard && route.Protocol != ProtocolExtended) {
			panic(fmt.Sprintf("无效市场定义 %q", route.ID))
		}
		if route.CodeSource > CodeListZHBReport {
			panic(fmt.Sprintf("市场 %s 的代码来源 %d 未注册", route.ID, route.CodeSource))
		}
		if route.CodeSource == CodeListZHBReport {
			if route.Report == nil || route.Report.Member == "" || route.Report.Parser == "" {
				panic(fmt.Sprintf("市场 %s 的报表来源缺少成员或解析器", route.ID))
			}
		}
		for _, symbol := range route.BareSymbols {
			symbol = strings.ToUpper(symbol)
			if _, exists := bareInstruments[symbol]; exists {
				panic(fmt.Sprintf("重复定义裸代码 %q", symbol))
			}
			bareInstruments[symbol] = route.Exchange
		}
		marketRoutes[route.ID] = route
		exchangeRoutes[route.Exchange] = registeredRoute{route, market, info.Prefix, info.PreferBareSymbol}
		markets = append(markets, route.ID)
	}
	slices.Sort(markets)
	instrumentExchanges = append(instrumentExchanges, info.Routes[0].Exchange)
	slices.SortFunc(instrumentExchanges, func(a, b Exchange) int {
		if d := len(b.String()) - len(a.String()); d != 0 {
			return d
		}
		return strings.Compare(a.String(), b.String())
	})
}

// Markets returns the built-in configuration and storage keys in lexical order.
func Markets() []ID { return slices.Clone(markets) }

// StandardExchanges returns exchanges supported by the standard wire protocol.
func StandardExchanges() []Exchange {
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
	route, ok := marketRoutes[m]
	return route.Exchange, ok
}

// DiscoversBlocks reports whether this market includes sector indices from block_zs.dat.
func (m ID) DiscoversBlocks() bool { return marketRoutes[m].DiscoverBlocks }

// Info returns the logical market and all of its protocol routes.
func (m ID) Info() (MarketInfo, bool) {
	if route, ok := marketRoutes[m]; ok {
		return exchangeRoutes[route.Exchange].market.Info(), true
	}
	return MarketInfo{}, false
}

// Route returns the protocol metadata associated with the wire exchange.
func (e Exchange) Route() (RouteInfo, bool) {
	route, ok := exchangeRoutes[e]
	return route.RouteInfo, ok
}
