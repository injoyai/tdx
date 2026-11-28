package market

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
)

// Definition describes a 7727 market using the existing K-line layouts.
// Key is both the code prefix and storage key. Code case is preserved.
type Definition struct {
	Key               ID
	Exchange          Exchange
	CodePattern       string // Full-code RE2 pattern.
	Security          SecurityKind
	DayVolumeScale    int64 // 0 leaves volume unavailable.
	MinuteVolumeScale int64
	AmountScale       float64 // Shared amount scale, used when both period scales are zero.
	DayAmountScale    float64 `json:",omitempty"`
	MinuteAmountScale float64 `json:",omitempty"`
	TimeZone          string  `json:",omitempty"`
	CloseTime         string  `json:",omitempty"`
}

type customMarket struct {
	def     Definition
	pattern *regexp.Regexp
}

// New constructs a 7727 market with the existing paged day and one-minute layouts.
func New(def Definition) (Market, error) {
	if !validMarketKey(def.Key.String()) {
		return nil, fmt.Errorf("无效市场键 %q", def.Key)
	}
	if def.CodePattern == "" {
		return nil, fmt.Errorf("市场 %s 缺少代码格式", def.Key)
	}
	pattern, err := regexp.Compile(def.CodePattern)
	if err != nil {
		return nil, err
	}
	pattern.Longest()
	if def.DayVolumeScale < 0 || def.MinuteVolumeScale < 0 ||
		!validAmountScale(def.AmountScale) || !validAmountScale(def.DayAmountScale) || !validAmountScale(def.MinuteAmountScale) ||
		(def.AmountScale != 0 && (def.DayAmountScale != 0 || def.MinuteAmountScale != 0)) {
		return nil, fmt.Errorf("市场 %s 的量额倍率无效", def.Key)
	}
	if def.Security == "" {
		def.Security = SecurityOther
	}
	switch def.Security {
	case SecurityOther, SecurityStock, SecurityIndex, SecurityFund, SecurityBond, SecurityBlock, SecurityFuture:
	default:
		return nil, fmt.Errorf("市场 %s 的品种分类无效", def.Key)
	}
	return customMarket{def: def, pattern: pattern}, nil
}

func validAmountScale(scale float64) bool {
	return scale >= 0 && scale <= math.MaxFloat64/math.MaxFloat32
}

func (m customMarket) Info() MarketInfo {
	name := m.def.Key.String()
	if slices.Contains(rawExchanges, m.def.Exchange) {
		name = m.def.Exchange.rawName()
	}
	return MarketInfo{
		Prefix: m.def.Key.String(), Name: name, PreferBareSymbol: true,
		TimeZone: m.def.TimeZone, CloseTime: m.def.CloseTime,
		Routes: []RouteInfo{{
			ID: m.def.Key, Exchange: m.def.Exchange, Name: name,
			Protocol: ProtocolExtended, CodeSource: CodeListExtended,
		}},
	}
}

func (m customMarket) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	_, _, valid := symbolParts(code.Symbol, "._-")
	if valid {
		match := m.pattern.FindStringIndex(code.Symbol)
		valid = match != nil && match[0] == 0 && match[1] == len(code.Symbol)
	}
	return validateInstrument(m.def.Exchange, code, valid)
}

func (m customMarket) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, m.def.Exchange); err != nil {
		return InstrumentRules{}, err
	}
	r := ExtendedRules(m.def.Security)
	r.KlinePeriods = 1<<TypeKlineDay2 | 1<<TypeKlineMinute2
	r.RangePeriods = 0
	r.Day.VolumeScale, r.Minute.VolumeScale = m.def.DayVolumeScale, m.def.MinuteVolumeScale
	r.DayFields.Volume, r.IntradayFields.Volume = m.def.DayVolumeScale > 0, m.def.MinuteVolumeScale > 0
	r.DayAmountScale, r.MinuteAmountScale = m.def.DayAmountScale, m.def.MinuteAmountScale
	if m.def.AmountScale > 0 {
		r.DayAmountScale, r.MinuteAmountScale = m.def.AmountScale, m.def.AmountScale
	}
	r.DayFields.Amount, r.IntradayFields.Amount = r.DayAmountScale > 0, r.MinuteAmountScale > 0
	return r, nil
}

func (m customMarket) identity() string {
	def := m.def
	def.TimeZone, def.CloseTime = "", ""
	data, _ := json.Marshal(def)
	return string(data)
}
