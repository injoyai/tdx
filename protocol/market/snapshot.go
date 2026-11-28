package market

import (
	"fmt"
	"strconv"
	"strings"
)

// InstrumentSnapshot is a read-only index of instruments used for market routing.
type InstrumentSnapshot struct{ items map[Instrument]struct{} }

func NewInstrumentSnapshot(items []Instrument) InstrumentSnapshot {
	index := make(map[Instrument]struct{}, len(items))
	for _, item := range items {
		index[item] = struct{}{}
	}
	return InstrumentSnapshot{items: index}
}

// Loaded reports whether an index has been built from a discovery result.
func (s InstrumentSnapshot) Loaded() bool { return s.items != nil }

type snapshotRequired struct{ lookup InstrumentLookup }

func (e *snapshotRequired) Error() string {
	return fmt.Sprintf("%s 缺少通达信品种快照（候选市场 %s）", e.lookup, e.lookup.Candidates())
}

// Resolve returns the unique instrument in the candidate markets.
func (s InstrumentSnapshot) Resolve(lookup InstrumentLookup) (Instrument, error) {
	if len(lookup.Exchanges) == 0 {
		return Instrument{}, fmt.Errorf("%s 没有指定品种快照候选市场", lookup)
	}
	if !s.Loaded() {
		return Instrument{}, &snapshotRequired{lookup: lookup}
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
