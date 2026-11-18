package market

// ExtendedTradeNature selects the decoder for an extended trade direction field.
func ExtendedTradeNature(ex Exchange) func(uint16, uint32, int32) (int, string) {
	switch ex {
	case 48: // 旧港股协议市场
		return hkTradeNature
	case ExchangeCFFO, ExchangeCZCO, ExchangeDCEO, ExchangeSHFO, ExchangeGFEO, ExchangeSHO, ExchangeSZO:
		return futuresTradeNature
	default:
		return marketDefinitions[exchangeMarkets[ex]].tradeNature
	}
}

func hkTradeNature(direction uint16, _ uint32, _ int32) (int, string) {
	switch direction {
	case 0:
		return 1, "B"
	case 256:
		return -1, "S"
	default:
		return 0, ""
	}
}

// futuresTradeNature 还原买卖方向与开平性质，移植自 pytdx。
func futuresTradeNature(direction uint16, volume uint32, zengcang int32) (int, string) {
	value := int(direction) / 10000
	vol := int32(volume)
	switch value {
	case 0:
		if zengcang > 0 {
			if vol > zengcang {
				return 1, "多开"
			}
			if vol == zengcang {
				return 1, "双开"
			}
			return 1, "多开"
		} else if zengcang == 0 {
			return 1, "多换"
		}
		if vol == -zengcang {
			return 1, "双平"
		}
		return 1, "空平"
	case 1:
		if zengcang > 0 {
			if vol > zengcang {
				return -1, "空开"
			}
			if vol == zengcang {
				return -1, "双开"
			}
			return -1, "空开"
		} else if zengcang == 0 {
			return -1, "空换"
		}
		if vol == -zengcang {
			return -1, "双平"
		}
		return -1, "多平"
	default:
		if zengcang > 0 {
			if vol > zengcang {
				return 0, "开仓"
			}
			if vol == zengcang {
				return 0, "双开"
			}
			return 0, "开仓"
		} else if zengcang < 0 {
			if vol > -zengcang {
				return 0, "平仓"
			}
			if vol == -zengcang {
				return 0, "双平"
			}
			return 0, "平仓"
		}
		return 0, "换手"
	}
}
