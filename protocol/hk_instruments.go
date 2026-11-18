package protocol

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol/market"
)

// HKCode extracts the numeric code in a shared Hong Kong symbol. An explicit
// wire exchange such as 00823.49 identifies a market without a snapshot.
func HKCode(input string) (string, bool) { return market.HKCode(input) }

const (
	hkStock = 1 << iota
	hkFund
)

// HKInstruments is an immutable index of markets 31 and 49 from a TDX instrument snapshot.
type HKInstruments struct{ codes map[string]uint8 }

// NewHKInstruments indexes the complete instrument snapshot returned by GetInstruments.
func NewHKInstruments(items []ExInstrument) HKInstruments {
	index := HKInstruments{codes: make(map[string]uint8)}
	for _, item := range items {
		switch Exchange(item.Market) {
		case ExchangeHK:
			index.codes[item.Code] |= hkStock
		case ExchangeHKFund:
			index.codes[item.Code] |= hkFund
		}
	}
	return index
}

// Loaded reports whether the index has been constructed from a snapshot.
func (s HKInstruments) Loaded() bool { return s.codes != nil }

// Exchange returns the unique wire market for a bare Hong Kong code.
func (s HKInstruments) Exchange(code string) (Exchange, error) {
	switch s.codes[code] {
	case hkStock:
		return ExchangeHK, nil
	case hkFund:
		return ExchangeHKFund, nil
	case hkStock | hkFund:
		return 0, fmt.Errorf("hk.%s 同时出现在通达信市场 31 和 49", code)
	default:
		if !s.Loaded() {
			return 0, fmt.Errorf("hk.%s 缺少通达信品种快照（候选市场 31、49）", code)
		}
		return 0, fmt.Errorf("hk.%s 在通达信品种快照中未找到（候选市场 31、49）", code)
	}
}
