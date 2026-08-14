package protocol

import (
	"fmt"
	"strconv"
	"strings"
)

type Control uint8

func (this Control) Uint8() uint8 {
	return uint8(this)
}

const (
	Control01 Control = 0x01 //好像都是01，暂时不知道啥含义
)

type Exchange uint8

func (this Exchange) Uint8() uint8 { return uint8(this) }

func (this Exchange) String() string {
	switch this {
	case ExchangeSZ:
		return "sz"
	case ExchangeSH:
		return "sh"
	case ExchangeBJ:
		return "bj"
	case ExchangeNQ:
		return "nq"
	case ExchangeSHO:
		return "sho"
	case ExchangeSZO:
		return "szo"
	case ExchangeHK:
		return "hk"
	case ExchangeUS:
		return "us"
	case ExchangeCSI:
		return "csi"
	case ExchangeCNI:
		return "cni"
	case ExchangeHG:
		return "hg"
	case ExchangeCFF:
		return "cff"
	case ExchangeCZC:
		return "czc"
	case ExchangeDCE:
		return "dce"
	case ExchangeSHF:
		return "shf"
	case ExchangeGFE:
		return "gfe"
	case ExchangeHI:
		return "hi"
	case ExchangeOF:
		return "of"
	case ExchangeCFFO:
		return "cffo"
	case ExchangeCZCO:
		return "czco"
	case ExchangeDCEO:
		return "dceo"
	case ExchangeSHFO:
		return "shfo"
	case ExchangeGFEO:
		return "gfeo"
	case ExchangeQHZ:
		return "qhz"
	default:
		return "unknown"
	}
}

func (this Exchange) Name() string {
	switch this {
	case ExchangeSH:
		return "上海"
	case ExchangeSZ:
		return "深圳"
	case ExchangeBJ:
		return "北京"
	case ExchangeNQ:
		return "新三板"
	case ExchangeSHO:
		return "上海个股期权"
	case ExchangeSZO:
		return "深圳个股期权"
	case ExchangeHK:
		return "香港交易所"
	case ExchangeUS:
		return "美国股票"
	case ExchangeCSI:
		return "中证指数"
	case ExchangeCNI:
		return "国证指数"
	case ExchangeHG:
		return "国内宏观指标"
	case ExchangeCFF:
		return "中金期货"
	case ExchangeCZC:
		return "郑州期货"
	case ExchangeDCE:
		return "大连期货"
	case ExchangeSHF:
		return "上海期货"
	case ExchangeGFE:
		return "广州期货"
	case ExchangeHI:
		return "港股指数"
	case ExchangeOF:
		return "开放式基金净值"
	case ExchangeCFFO:
		return "中金所期权"
	case ExchangeCZCO:
		return "郑州期货期权"
	case ExchangeDCEO:
		return "大连期货期权"
	case ExchangeSHFO:
		return "上海期货期权"
	case ExchangeGFEO:
		return "广州期货期权"
	case ExchangeQHZ:
		return "期货类指数"
	default:
		return "未知"
	}
}

// ParseExchange 把字符串转为Exchange类型,支持:
//   - 数字字符串: "0"/"1"/"2"/"44" 等,对应市场编码数值
//   - 小写缩写: "sz"/"sh"/"bj"/"nq" 等,对应 String() 输出
//   - 中文名称: "深圳"/"上海"/"北京"/"新三板" 等,对应 Name() 输出
//
// 无法识别时返回 error。
func ParseExchange(s string) (Exchange, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	// 数值形式,如 "0" "2" "44"
	if n, err := strconv.Atoi(s); err == nil {
		return Exchange(n), nil
	}
	for _, e := range allExchanges {
		if s == strings.ToLower(e.String()) || s == strings.ToLower(e.Name()) {
			return e, nil
		}
	}
	return 0, fmt.Errorf("无法识别的市场: %q", s)
}

// allExchanges 返回全部已知市场枚举
var allExchanges = []Exchange{
	ExchangeSZ, ExchangeSH, ExchangeBJ,
	ExchangeCFFO, ExchangeCZCO, ExchangeDCEO, ExchangeSHFO,
	ExchangeCZC, ExchangeDCE, ExchangeSHF, ExchangeGFEO, ExchangeGFE,
	ExchangeHI, ExchangeOF, ExchangeSZO, ExchangeSHO,
	ExchangeCFF, ExchangeQHZ, ExchangeNQ,
	ExchangeHK, ExchangeCSI, ExchangeHG, ExchangeUS, ExchangeCNI,
}

// isExchangeName 判断字符串是否为已知市场的字母缩写或中文名(排除纯数字)。
// 用于 DecodeCode 校验前缀,避免 "000001.SZ" 之类输入把数字前缀 "000" 误当市场编码。
func isExchangeName(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, e := range allExchanges {
		if s == strings.ToLower(e.String()) || s == strings.ToLower(e.Name()) {
			return true
		}
	}
	return false
}

// 市场类型, 来源: 通达信量化 Dict 枚举
// https://help.tdx.com.cn/quant/docs/markdown/Dict.html
const (
	ExchangeSZ Exchange = iota //深圳交易所
	ExchangeSH                 //上海交易所
	ExchangeBJ                 //北京交易所

	// 以下为通达信量化 API 补充的市场类型
	ExchangeNQ   Exchange = 44  //新三板
	ExchangeSHO  Exchange = 8   //上海个股期权
	ExchangeSZO  Exchange = 9   //深圳个股期权
	ExchangeHK   Exchange = 31  //香港交易所
	ExchangeUS   Exchange = 74  //美国股票
	ExchangeCSI  Exchange = 62  //中证指数
	ExchangeCNI  Exchange = 102 //国证指数
	ExchangeHG   Exchange = 38  //国内宏观指标
	ExchangeCFF  Exchange = 47  //中金期货
	ExchangeCZC  Exchange = 28  //郑州期货
	ExchangeDCE  Exchange = 29  //大连期货
	ExchangeSHF  Exchange = 30  //上海期货
	ExchangeINE  Exchange = 30  //上海能源(与ExchangeSHF同值,代码同为30)
	ExchangeGFE  Exchange = 66  //广州期货
	ExchangeHI   Exchange = 27  //港股指数
	ExchangeOF   Exchange = 33  //开放式基金净值
	ExchangeCFFO Exchange = 7   //中金所期权
	ExchangeCZCO Exchange = 4   //郑州期货期权
	ExchangeDCEO Exchange = 5   //大连期货期权
	ExchangeSHFO Exchange = 6   //上海期货期权
	ExchangeGFEO Exchange = 67  //广州期货期权
	ExchangeQHZ  Exchange = 42  //期货类指数
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
	TypeKline5Minute  uint8 = 0  // 5分钟K 线
	TypeKline15Minute uint8 = 1  // 15分钟K 线
	TypeKline30Minute uint8 = 2  // 30分钟K 线
	TypeKline60Minute uint8 = 3  // 60分钟K 线
	TypeKlineHour     uint8 = 3  // 1小时K 线
	TypeKlineDay2     uint8 = 4  // 日K 线, 发现和Day的区别是这个要除以100,其他未知
	TypeKlineWeek     uint8 = 5  // 周K 线
	TypeKlineMonth    uint8 = 6  // 月K 线
	TypeKlineMinute   uint8 = 7  // 1分钟
	TypeKlineMinute2  uint8 = 8  // 1分钟K 线,未知啥区别
	TypeKlineDay      uint8 = 9  // 日K 线
	TypeKlineQuarter  uint8 = 10 // 季K 线
	TypeKlineYear     uint8 = 11 // 年K 线
)

const (
	KindIndex = "index"
	KindStock = "stock"
)
