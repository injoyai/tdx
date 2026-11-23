// 本文件仅在跨包验收时通过 go test -overlay 挂载为 protocol/market/zzq.go。
// 目录以下划线开头，正常构建与 go test ./... 不会包含它。
//
// 与 zzq.go 的差异仅为启用标准分笔能力，用于 M01 竞价金额回归：
// 拆竞价依赖分笔，默认夹具声明分笔未接入，无法构造真实拆分路径。
package market

import "fmt"

const (
	MarketZZQ   ID       = "zzq"
	ExchangeZZQ Exchange = 200
)

type zzq struct{}

func (*zzq) Info() MarketInfo {
	return MarketInfo{
		Prefix:    "zzq",
		Name:      "验收测试市场",
		TimeZone:  "Asia/Shanghai",
		CloseTime: "15:00",
		Routes: []RouteInfo{{
			ID:       MarketZZQ,
			Exchange: ExchangeZZQ,
			Name:     "验收测试",
			Protocol: ProtocolStandard,
			// 六位数字代码走标准代码列表来源。
			CodeSource: CodeListStandard,
			Session:    DomesticSession,
		}},
	}
}

func (*zzq) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	if err := checkExchange(code.Exchange, ExchangeZZQ); err != nil {
		return Instrument{}, err
	}
	valid := len(code.Symbol) == 6
	for _, c := range code.Symbol {
		valid = valid && c >= '0' && c <= '9'
	}
	return validateInstrument(ExchangeZZQ, code, valid)
}

func (*zzq) Rules(i Instrument) (InstrumentRules, error) {
	if i.Exchange != ExchangeZZQ {
		return InstrumentRules{}, fmt.Errorf("行情市场 %d 不属于 %s", i.Exchange, MarketZZQ)
	}
	info := standardSecurityInfo(SecurityStock)
	// 分钟、日线、分时与分笔统一按 10 倍业务量换算。
	info.Minute.VolumeScale = 10
	info.Day.VolumeScale = 10
	info.setTickVolume(10)
	// 只开启拆竞价，不允许用分笔补分钟；分笔用于竞价拆分。
	info.SplitAuction = true
	info.TradeFallback = false
	return info, nil
}

func init() { registerMarket(&zzq{}) }
