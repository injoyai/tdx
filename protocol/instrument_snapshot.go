package protocol

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/injoyai/tdx/v2/protocol/market"
)

type InstrumentLookup = market.InstrumentLookup

// LookupInstrument returns the snapshot lookup described by a market definition.
func LookupInstrument(input string) (InstrumentLookup, bool) { return market.LookupInstrument(input) }

// InstrumentSnapshot indexes codes in the wire markets that use snapshot routing.
type InstrumentSnapshot struct{ items map[Instrument]struct{} }

// NewInstrumentSnapshot indexes the completed discovery result returned by GetInstruments.
func NewInstrumentSnapshot(items []ExInstrument) InstrumentSnapshot {
	snapshot := InstrumentSnapshot{items: make(map[Instrument]struct{})}
	for _, item := range items {
		ex := Exchange(item.Market)
		if market.UsesInstrumentSnapshot(ex) {
			snapshot.items[Instrument{Exchange: ex, Code: item.Code}] = struct{}{}
		}
	}
	return snapshot
}

// Loaded reports whether an index has been built from a discovery result.
func (s InstrumentSnapshot) Loaded() bool { return s.items != nil }

// Resolve returns the unique instrument in the candidate markets.
func (s InstrumentSnapshot) Resolve(lookup InstrumentLookup) (Instrument, error) {
	if !s.Loaded() {
		return Instrument{}, fmt.Errorf("%s 缺少通达信品种快照（候选市场 %s）", lookup, lookup.Candidates())
	}
	var matches []string
	var found Instrument
	for _, ex := range lookup.Exchanges {
		i := Instrument{Exchange: ex, Code: lookup.Code}
		if _, ok := s.items[i]; ok {
			found = i
			matches = append(matches, strconv.Itoa(int(ex)))
		}
	}
	switch len(matches) {
	case 1:
		return found, nil
	case 0:
		return Instrument{}, fmt.Errorf("%s 在通达信品种快照中未找到（候选市场 %s）", lookup, lookup.Candidates())
	default:
		return Instrument{}, fmt.Errorf("%s 同时出现在通达信市场 %s", lookup, strings.Join(matches, " 和 "))
	}
}
