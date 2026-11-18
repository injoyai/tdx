package market

const (
	MarketSH   Market   = "sh"
	ExchangeSH Exchange = 1 // 标准行情
)

func init() {
	def := standardDefinition(ExchangeSH, "sh", "上海",
		securityRule{SecurityBlock, isBlock},
		securityRule{SecurityFund, isSHETF},
		securityRule{SecurityIndex, isSHIndex},
		securityRule{SecurityBond, isSHBond},
		securityRule{SecurityStock, isSHStock},
	)
	def.discoverBlocks = true
	registerMarket(MarketSH, def)
}
