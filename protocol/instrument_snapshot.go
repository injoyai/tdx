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

// LoadInstrumentMarkets configures the directory and loads saved market assignments.
func LoadInstrumentMarkets(dir string) error { return market.LoadInstrumentMarkets(dir) }

// ImportInstruments imports a completed GetInstruments result into the markets.
func ImportInstruments(items []ExInstrument) error {
	instruments := make([]Instrument, 0, len(items))
	for _, item := range items {
		instruments = append(instruments, Instrument{Exchange: Exchange(item.Market), Code: item.Code})
	}
	return market.ImportInstruments(instruments)
}

// ResolveInstrument parses a code using a supplied snapshot or the market's known assignments.
func ResolveInstrument(input string, load func(InstrumentLookup) (InstrumentSnapshot, error)) (Instrument, error) {
	return market.ResolveInstrument(input, load)
}

// QuoteProtocol selects the connection for a code. Shared codes are resolved
// by the selected connection before an instrument request. A shared code whose
// candidate routes disagree on the protocol must be given an explicit market.
func QuoteProtocol(code string) (MarketProtocol, error) {
	if lookup, needed := LookupInstrument(code); needed {
		var selected MarketProtocol
		for _, ex := range lookup.Exchanges {
			route, ok := ex.Route()
			if !ok {
				return "", fmt.Errorf("候选行情市场 %d 尚未接入统一 Client", ex)
			}
			if selected == "" {
				selected = route.Protocol
			} else if route.Protocol != selected {
				return "", fmt.Errorf("%s 的候选市场协议不一致（%s），请显式指定市场", lookup, lookup.Candidates())
			}
		}
		return selected, nil
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
