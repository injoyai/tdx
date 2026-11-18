package protocol

// SharesPerLot 是协议百股单位的股数，不代表交易所每手的实际股数。
const SharesPerLot = 100

// BondsPerLot 是标准行情可转债分时、分笔、报价及分钟 K 线每手的张数。
const BondsPerLot = 10

// ToShares 将协议手数转换为股数；调用前须确认来源单位为百股。
func ToShares(lots int64) int64 { return lots * SharesPerLot }

// FromShares 将股数转换为整数手数，余数按 Go 整数除法截断。
func FromShares(shares int64) int64 { return shares / SharesPerLot }
