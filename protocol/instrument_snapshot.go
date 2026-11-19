package protocol

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol/market"
)

type (
	InstrumentLookup   = market.InstrumentLookup
	InstrumentSnapshot = market.InstrumentSnapshot
)

// LookupInstrument reports whether the user input requires a snapshot.
func LookupInstrument(input string) (InstrumentLookup, bool) { return market.LookupInstrument(input) }

// NewInstrumentSnapshot indexes the completed discovery result returned by GetInstruments.
func NewInstrumentSnapshot(items []ExInstrument) InstrumentSnapshot {
	var instruments []Instrument
	for _, item := range items {
		ex := Exchange(item.Market)
		if market.UsesInstrumentSnapshot(ex) {
			instruments = append(instruments, Instrument{Exchange: ex, Code: item.Code})
		}
	}
	return market.NewInstrumentSnapshot(instruments)
}

// ResolveInstrument parses a code and uses the caller's snapshot loader if required.
func ResolveInstrument(input string, load func(InstrumentLookup) (InstrumentSnapshot, error)) (Instrument, error) {
	return market.ResolveInstrument(input, load)
}

// QuoteProtocol selects the connection for a code. Shared codes are resolved
// against the connection's snapshot separately before an instrument request.
func QuoteProtocol(code string) (MarketProtocol, error) {
	if _, needed := LookupInstrument(code); needed {
		return ProtocolExtended, nil
	}
	i, err := ParseInstrument(code)
	if err != nil {
		return "", err
	}
	if route, known := i.Exchange.Route(); known {
		return route.Protocol, nil
	}
	return "", fmt.Errorf("行情市场 %d 尚未接入统一 Client", i.Exchange)
}
