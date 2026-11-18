package protocol

import "github.com/injoyai/tdx/v2/protocol/market"

type (
	Market         = market.Market
	MarketInfo     = market.MarketInfo
	MarketProtocol = market.MarketProtocol
	KlineInfo      = market.KlineInfo
	Exchange       = market.Exchange
	Instrument     = market.Instrument
	SecurityKind   = market.SecurityKind
)

const (
	MarketSH                 = market.MarketSH
	MarketSZ                 = market.MarketSZ
	MarketBJ                 = market.MarketBJ
	MarketHK                 = market.MarketHK
	MarketHKFund             = market.MarketHKFund
	MarketHKIndex            = market.MarketHKIndex
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
func Markets() []Market { return market.Markets() }

// StandardExchanges returns exchanges supported by the standard protocol.
func StandardExchanges() []Exchange { return market.StandardExchanges() }

// ParseExchange parses an exchange abbreviation, name, or wire identifier.
func ParseExchange(s string) (Exchange, error) { return market.ParseExchange(s) }

// ParseInstrument parses the exchange and bare code of a security.
func ParseInstrument(s string) (Instrument, error) { return market.ParseInstrument(s) }

func standardMarketInfo(code string) MarketInfo {
	if i, err := ParseInstrument(code); err == nil {
		if info, ok := i.Info(); ok && info.Protocol == ProtocolStandard {
			return info
		}
	}
	info, _ := Market("").Info()
	return info
}
