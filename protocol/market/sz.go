package market

const (
	MarketSZ   Market   = "sz"
	ExchangeSZ Exchange = 0 // 标准行情
)

func init() {
	def := standardDefinition(ExchangeSZ, "sz", "深圳",
		securityRule{SecurityFund, isSZETF},
		securityRule{SecurityIndex, isSZIndex},
		securityRule{SecurityBond, isSZBond},
		securityRule{SecurityStock, isSZStock},
	)
	registerMarket(MarketSZ, def)
}
