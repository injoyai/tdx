package protocol

import "time"

const (
	TypeConnect       = 0x000D //建立连接
	TypeHeart         = 0x0004 //心跳
	TypeGbbq          = 0x000F //除权除息
	TypeCount         = 0x044E //获取股票数量
	TypeCode          = 0x0450 //获取股票代码
	TypeQuote         = 0x053E //行情信息
	TypeMinute        = 0x051D //分时数据
	TypeCallAuction   = 0x056A //集合竞价
	TypeTrade         = 0x0FC5 //分时交易
	TypeHistoryMinute = 0x0FB4 //历史分时数据
	TypeHistoryTrade  = 0x0FB5 //历史分时交易
	TypeKline         = 0x052D //K线图
)

var (
	// ExchangeEstablish 交易所成立时间
	ExchangeEstablish = time.Date(1990, 12, 19, 0, 0, 0, 0, Location())
)
