package protocol

import "github.com/injoyai/tdx/v2/protocol/market"

type Control uint8

func (c Control) Uint8() uint8 {
	return uint8(c)
}

const (
	Control01 Control = 0x01 //好像都是01，暂时不知道啥含义
)

// OrderType 委托类型, 来源: 通达信量化 Dict 枚举
type OrderType uint8

const (
	OrderTypeStockBuy         OrderType = 0   //股票买
	OrderTypeStockSell        OrderType = 1   //股票卖
	OrderTypeCreditBuy        OrderType = 0   //担保品买入
	OrderTypeCreditSell       OrderType = 1   //担保品卖出
	OrderTypeCreditFinBuy     OrderType = 69  //融资买入
	OrderTypeCreditSloSell    OrderType = 70  //融券卖出
	OrderTypeCreditCovBuy     OrderType = 71  //买券还券
	OrderTypeCreditStkRepay   OrderType = 76  //卖券还款
	OrderTypeETFPurchase      OrderType = 45  //基金申购
	OrderTypeETFRedeem        OrderType = 46  //基金赎回
	OrderTypeFutureOpenLong   OrderType = 101 //期货开多
	OrderTypeFutureOpenShort  OrderType = 102 //期货开空
	OrderTypeFutureCloseLong  OrderType = 103 //期货平多
	OrderTypeFutureCloseShort OrderType = 104 //期货平空
	OrderTypeOptionOpenLong   OrderType = 201 //期权开多
	OrderTypeOptionOpenShort  OrderType = 202 //期权开空
	OrderTypeOptionCloseLong  OrderType = 203 //期权平多
	OrderTypeOptionCloseShort OrderType = 204 //期权平空
)

// PriceType 价格类型, 来源: 通达信量化 Dict 枚举
type PriceType uint8

const (
	PriceTypeMy  PriceType = 0 //自填价
	PriceTypeSj  PriceType = 1 //市价
	PriceTypeZtj PriceType = 2 //涨停价/笼子上限
	PriceTypeDtj PriceType = 3 //跌停价/笼子下限
)

// Status 委托状态, 来源: 通达信量化 Dict 枚举
type Status uint8

const (
	StatusNull   Status = 0 //无效单
	StatusNoCj   Status = 1 //未成交
	StatusPartCj Status = 2 //部分成交
	StatusAllCj  Status = 3 //全部成交
	StatusBcbc   Status = 4 //部分成交部分撤单
	StatusAllCd  Status = 5 //全部撤单
)

// DividendType 复权类型, 来源: 通达信量化 Dict 枚举
type DividendType string

const (
	DividendNone  DividendType = "none"  //不复权
	DividendFront DividendType = "front" //前复权
	DividendBack  DividendType = "back"  //后复权
)

const (
	TypeKline5Minute  = market.TypeKline5Minute
	TypeKline15Minute = market.TypeKline15Minute
	TypeKline30Minute = market.TypeKline30Minute
	TypeKline60Minute = market.TypeKline60Minute
	TypeKlineHour     = market.TypeKlineHour
	TypeKlineDay2     = market.TypeKlineDay2
	TypeKlineWeek     = market.TypeKlineWeek
	TypeKlineMonth    = market.TypeKlineMonth
	TypeKlineMinute   = market.TypeKlineMinute
	TypeKlineMinute2  = market.TypeKlineMinute2
	TypeKlineDay      = market.TypeKlineDay
	TypeKlineQuarter  = market.TypeKlineQuarter
	TypeKlineYear     = market.TypeKlineYear
	KindIndex         = market.KindIndex
	KindStock         = market.KindStock
)
