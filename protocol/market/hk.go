package market

import "strings"

const (
	MarketHK   Market   = "hk/stock"
	ExchangeHK Exchange = 31 // 扩展行情
)

func init() {
	def := hkEquityDefinition(ExchangeHK, "hk", "香港交易所", parseHK)
	def.security = SecurityStock
	def.parseExplicit = numericHKCode
	def.lookup = snapshotRule{code: hkNumericCode, exchanges: []Exchange{ExchangeHK, ExchangeHKFund}}
	registerMarket(MarketHK, def)
}

func hkEquityDefinition(ex Exchange, prefix, name string, parse func(Instrument) (Instrument, bool)) marketDefinition {
	def := extendedDefinition(ex, prefix, name, parse)
	def.info.TimeZone, def.info.CloseTime = "Asia/Hong_Kong", "16:10"
	def.info.TradePriceDivisor = 1_000
	def.info.Day.VolumeScale, def.info.Minute.VolumeScale = SharesPerLot, SharesPerLot
	def.info.LongVolumeScale, def.info.TickVolumeScale = 10_000, 1
	def.info.extendedKlineAmountScale = 1
	def.tradeNature = hkTradeNature
	return def
}

func parseHK(i Instrument) (Instrument, bool) {
	if index, ok := upperSymbol(i, ""); ok {
		index.Exchange = ExchangeHI
		return index, true
	}
	if _, ok := numericHKCode(i); !ok {
		return i, false
	}
	if isHKETFCode(i.Code) {
		i.Exchange = ExchangeHKFund
	}
	return i, true
}

// isHKETFCode 依据港交所证券代号编配计划匹配五位数字 ETF 代码。
// https://www.hkex.com.hk/Products/Securities/Stock-Code-Allocation-Plan
func isHKETFCode(code string) bool {
	for _, span := range [][2]string{
		{"02800", "02849"}, {"03000", "03199"}, {"03400", "03499"}, // 港元
		{"09000", "09199"}, {"09400", "09499"}, {"09800", "09849"}, // 美元
		{"82800", "82849"}, {"83000", "83199"}, {"83400", "83499"}, // 人民币
	} {
		if code >= span[0] && code <= span[1] {
			return true
		}
	}
	return false
}

func hkNumericCode(input string) (string, bool) {
	code := strings.TrimSpace(input)
	if len(code) >= 2 && strings.EqualFold(code[:2], "hk") {
		code = code[2:]
		if strings.HasPrefix(code, ".") || strings.HasPrefix(code, "/") {
			code = code[1:]
		}
	} else if body, suffix, ok := strings.Cut(code, "."); ok && strings.EqualFold(suffix, "hk") {
		code = body
	}
	if len(code) == 5 && isDigits(code) {
		return code, true
	}
	return "", false
}
