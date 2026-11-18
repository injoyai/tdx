package market

import (
	"fmt"
	"strconv"
	"strings"
)

type Exchange uint8

func (e Exchange) Uint8() uint8 { return uint8(e) }

// String 返回证券代码前缀。港股股票、基金和指数共用 hk。
func (e Exchange) String() string {
	if m, ok := exchangeMarkets[e]; ok {
		return marketDefinitions[m].prefix
	}
	switch e {
	case ExchangeNQ:
		return "nq"
	case ExchangeSHO:
		return "sho"
	case ExchangeSZO:
		return "szo"
	case ExchangeCSI:
		return "csi"
	case ExchangeCNI:
		return "cni"
	case ExchangeHG:
		return "hg"
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
	default:
		return "unknown"
	}
}

func (e Exchange) Name() string {
	if m, ok := exchangeMarkets[e]; ok {
		return marketDefinitions[m].name
	}
	switch e {
	case ExchangeNQ:
		return "新三板"
	case ExchangeSHO:
		return "上海个股期权"
	case ExchangeSZO:
		return "深圳个股期权"
	case ExchangeCSI:
		return "中证指数"
	case ExchangeCNI:
		return "国证指数"
	case ExchangeHG:
		return "国内宏观指标"
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
	default:
		return "未知"
	}
}

// ParseExchange 把字符串转为Exchange类型,支持:
//   - 数字字符串: "0"/"1"/"2"/"44" 等,对应市场编码数值
//   - 小写缩写: "sz"/"sh"/"bj"/"nq" 等,对应 String() 输出
//   - 中文名称: "深圳"/"上海"/"北京"/"新三板" 等,对应 Name() 输出
//
// 单独传入 hk 时返回港股主板；可用 27 或港股指数指定指数市场。
// 无法识别时返回 error。
func ParseExchange(s string) (Exchange, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	// 数值形式,如 "0" "2" "44"
	if n, err := strconv.Atoi(s); err == nil {
		return Exchange(n), nil
	}
	for _, e := range allExchanges() {
		if s == strings.ToLower(e.String()) || s == strings.ToLower(e.Name()) {
			return e, nil
		}
	}
	return 0, fmt.Errorf("无法识别的市场: %q", s)
}

// Known exchange names include raw protocol exchanges with no unified market.
var rawExchanges = []Exchange{
	ExchangeNQ, ExchangeSHO, ExchangeSZO, ExchangeCFFO, ExchangeCZCO,
	ExchangeDCEO, ExchangeSHFO, ExchangeGFEO, ExchangeOF,
	ExchangeCSI, ExchangeHG, ExchangeCNI,
}

func allExchanges() []Exchange {
	out := make([]Exchange, 0, len(rawExchanges)+len(markets))
	out = append(out, rawExchanges...)
	out = append(out, instrumentExchanges...)
	for _, m := range markets {
		def := marketDefinitions[m]
		if def.sharedPrefix {
			out = append(out, def.exchange)
		}
	}
	return out
}

// isExchangeName 判断字符串是否为已知市场的字母缩写或中文名(排除纯数字)。
// 用于 DecodeCode 校验前缀,避免 "000001.SZ" 之类输入把数字前缀 "000" 误当市场编码。
func isExchangeName(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, e := range allExchanges() {
		if s == strings.ToLower(e.String()) || s == strings.ToLower(e.Name()) {
			return true
		}
	}
	return false
}

// 市场类型, 来源: 通达信量化 Dict 枚举
// https://help.tdx.com.cn/quant/docs/markdown/Dict.html
const (
	ExchangeNQ  Exchange = 44 //新三板
	ExchangeSHO Exchange = 8  //上海个股期权
	ExchangeSZO Exchange = 9  //深圳个股期权

	ExchangeCSI Exchange = 62  //中证指数
	ExchangeCNI Exchange = 102 //国证指数
	ExchangeHG  Exchange = 38  //国内宏观指标

	ExchangeOF   Exchange = 33 //开放式基金净值
	ExchangeCFFO Exchange = 7  //中金所期权
	ExchangeCZCO Exchange = 4  //郑州期货期权
	ExchangeDCEO Exchange = 5  //大连期货期权
	ExchangeSHFO Exchange = 6  //上海期货期权
	ExchangeGFEO Exchange = 67 //广州期货期权

)
