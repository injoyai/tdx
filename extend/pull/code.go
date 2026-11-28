package pull

import (
	"fmt"
	"strings"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// ParseCode resolves a market-qualified code such as us.ANT or cff.IF2609.
// SH, SZ, BJ and HK also accept compact forms. Numeric HK codes may need a snapshot.
func ParseCode(s string) (model.Code, error) {
	return ParseCodeWithSnapshot(s, protocol.InstrumentSnapshot{})
}

// ParseCodeWithSnapshot resolves shared market symbols against an instrument snapshot.
func ParseCodeWithSnapshot(s string, snapshot protocol.InstrumentSnapshot) (model.Code, error) {
	return parseCode(s, func(protocol.InstrumentLookup) (protocol.InstrumentSnapshot, error) { return snapshot, nil })
}

func parseCode(s string, resolve func(protocol.InstrumentLookup) (protocol.InstrumentSnapshot, error)) (model.Code, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return model.Code{}, fmt.Errorf("pull: 空代码")
	}
	// Unit names can contain dots, so prefer the longest matching name.
	for p := strings.LastIndexByte(s, '.'); p > 0; p = strings.LastIndexByte(s[:p], '.') {
		u, ok := Get(s[:p])
		if !ok {
			continue
		}
		market := protocol.MarketID(u.Name())
		if _, wired := market.Exchange(); wired {
			continue
		}
		if p == len(s)-1 {
			return model.Code{}, fmt.Errorf("pull: 市场 %s 缺少代码", market)
		}
		return model.Code{Market: market, Code: s[p+1:]}, nil
	}
	i, err := protocol.ResolveInstrument(s, resolve)
	if err != nil {
		return model.Code{}, err
	}
	market, err := i.Market()
	if err != nil {
		return model.Code{}, err
	}
	return model.Code{Market: market, Code: i.StorageCode()}, nil
}
