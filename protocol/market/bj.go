package market

const (
	MarketBJ   Market   = "bj"
	ExchangeBJ Exchange = 2 // 标准行情
)

func init() {
	def := standardDefinition(ExchangeBJ, "bj", "北京",
		securityRule{SecurityIndex, isBJIndex},
		securityRule{SecurityStock, isBJStock},
	)
	registerMarket(MarketBJ, def)
}
