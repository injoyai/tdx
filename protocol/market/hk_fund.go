package market

const (
	MarketHKFund   Market   = "hk/fund"
	ExchangeHKFund Exchange = 49 // 扩展行情
)

func init() {
	def := hkEquityDefinition(ExchangeHKFund, "hk", "香港基金", numericHKCode)
	def.sharedPrefix, def.security = true, SecurityFund
	registerMarket(MarketHKFund, def)
}
