package market

import "strings"

const (
	MarketHKFund   Market   = "hk/fund"
	ExchangeHKFund Exchange = 49 // 扩展行情
)

func init() {
	def := hkEquityDefinition(ExchangeHKFund, "hk", "香港基金", numericHKCode)
	def.sharedPrefix, def.security = true, SecurityFund
	registerMarket(MarketHKFund, def)
}

// retiredHKFundCode rejects the former fund abbreviation before trying other markets.
func retiredHKFundCode(s string) bool {
	if len(s) <= 3 || !strings.EqualFold(s[:3], "hkf") {
		return false
	}
	body := s[3:]
	return body[0] == '.' || body[0] == '/' || (len(body) == 5 && isDigits(body))
}
