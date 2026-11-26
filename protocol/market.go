package protocol

import "github.com/injoyai/tdx/v2/protocol/market"

type (
	MarketID        = market.ID
	MarketInfo      = market.MarketInfo
	InstrumentRules = market.InstrumentRules
	KlineRule       = market.KlineRule
	KlineMode       = market.KlineMode
	TickRule        = market.TickRule
	TradeNature     = market.TradeNature
	QuoteRule       = market.QuoteRule
	Fields          = market.Fields
	Periods         = market.Periods
	MarketProtocol  = market.MarketProtocol
	KlineInfo       = market.KlineInfo
	Exchange        = market.Exchange
	Instrument      = market.Instrument
	SecurityKind    = market.SecurityKind
	TradingSession  = market.TradingSession
	CodeReport      = market.CodeReport
	ReportCode      = market.ReportCode
)

var DomesticSession = market.DomesticSession

const (
	TradeNeutral             = market.TradeNeutral
	TradeHK                  = market.TradeHK
	TradeFutures             = market.TradeFutures
	AllPeriods               = market.AllPeriods
	KlinePage                = market.KlinePage
	KlineRange               = market.KlineRange
	NoKlineAmount            = market.NoKlineAmount
	AmountInAmount           = market.AmountInAmount
	AmountInPosition         = market.AmountInPosition
	CodeListZHBReport        = market.CodeListZHBReport
	CodeListStandard         = market.CodeListStandard
	CodeListExtended         = market.CodeListExtended
	ReportTdxBjMore          = market.ReportTdxBjMore
	MarketSH                 = market.MarketSH
	MarketSZ                 = market.MarketSZ
	MarketBJ                 = market.MarketBJ
	MarketHK                 = market.MarketHK
	MarketHKFund             = market.MarketHKFund
	MarketHKIndex            = market.MarketHKIndex
	MarketCSI                = market.MarketCSI
	MarketUS                 = market.MarketUS
	MarketInternationalIndex = market.MarketInternationalIndex
	MarketCFF                = market.MarketCFF
	MarketCZC                = market.MarketCZC
	MarketDCE                = market.MarketDCE
	MarketSHF                = market.MarketSHF
	MarketGFE                = market.MarketGFE
	MarketQHZ                = market.MarketQHZ
	ExchangeBJ               = market.ExchangeBJ
	ExchangeCFF              = market.ExchangeCFF
	ExchangeCFFO             = market.ExchangeCFFO
	ExchangeCNI              = market.ExchangeCNI
	ExchangeCSI              = market.ExchangeCSI
	ExchangeCZC              = market.ExchangeCZC
	ExchangeCZCO             = market.ExchangeCZCO
	ExchangeDCE              = market.ExchangeDCE
	ExchangeDCEO             = market.ExchangeDCEO
	ExchangeGFE              = market.ExchangeGFE
	ExchangeGFEO             = market.ExchangeGFEO
	ExchangeHG               = market.ExchangeHG
	ExchangeHI               = market.ExchangeHI
	ExchangeHK               = market.ExchangeHK
	ExchangeHKFund           = market.ExchangeHKFund
	ExchangeINE              = market.ExchangeINE
	ExchangeNQ               = market.ExchangeNQ
	ExchangeOF               = market.ExchangeOF
	ExchangeQHZ              = market.ExchangeQHZ
	ExchangeSH               = market.ExchangeSH
	ExchangeSHF              = market.ExchangeSHF
	ExchangeSHFO             = market.ExchangeSHFO
	ExchangeSHO              = market.ExchangeSHO
	ExchangeSZ               = market.ExchangeSZ
	ExchangeSZO              = market.ExchangeSZO
	ExchangeUS               = market.ExchangeUS
	ExchangeWI               = market.ExchangeWI
	ProtocolExtended         = market.ProtocolExtended
	ProtocolStandard         = market.ProtocolStandard
	SecurityBlock            = market.SecurityBlock
	SecurityBond             = market.SecurityBond
	SecurityFund             = market.SecurityFund
	SecurityFuture           = market.SecurityFuture
	SecurityIndex            = market.SecurityIndex
	SecurityOther            = market.SecurityOther
	SecurityStock            = market.SecurityStock
)

// Markets returns the registered market keys.
func Markets() []MarketID { return market.Markets() }

// StandardExchanges returns exchanges supported by the standard protocol.
func StandardExchanges() []Exchange { return market.StandardExchanges() }

// ParseExchange parses an exchange abbreviation, name, or wire identifier.
func ParseExchange(s string) (Exchange, error) { return market.ParseExchange(s) }

// ParseInstrument parses the exchange and bare code of a security.
func ParseInstrument(s string) (Instrument, error) { return market.ParseInstrument(s) }

func standardMarketInfo(code string) InstrumentRules {
	if i, err := ParseInstrument(code); err == nil {
		if info, ok := i.Rules(); ok && info.Protocol == ProtocolStandard {
			return info
		}
	}
	return market.StandardRules()
}
