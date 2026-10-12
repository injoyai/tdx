package tdx

import (
	"errors"

	"github.com/injoyai/tdx/v2/protocol"
)

func standardKline(k *protocol.Kline) Kline {
	amount := k.Amount.Float64()
	return Kline{Time: k.Time.Format("2006-01-02 15:04"), Open: k.Open.Float64(), High: k.High.Float64(), Low: k.Low.Float64(), Close: k.Close.Float64(), Volume: k.Volume, Amount: &amount, Standard: k}
}

// StandardResponse 供标准指标、复权和旧 HTTP 数据契约使用，不换算扩展数据。
func (r *KlineResp) StandardResponse() (*protocol.KlineResp, error) {
	if r.Instrument.Exchange > protocol.ExchangeBJ {
		return nil, errors.New("扩展 K线不能转换为标准协议数据")
	}
	if len(r.List) > 65535 {
		return nil, errors.New("累计记录数超过标准返回类型 uint16 上限")
	}
	out := &protocol.KlineResp{Count: uint16(len(r.List))}
	for _, k := range r.List {
		if k.Standard == nil {
			return nil, errors.New("缺少标准 K线原始数据")
		}
		out.List = append(out.List, k.Standard)
	}
	return out, nil
}

func (r *TradeResp) StandardResponse() (*protocol.TradeResp, error) {
	if r.Instrument.Exchange > protocol.ExchangeBJ {
		return nil, errors.New("扩展分笔不能转换为标准协议数据")
	}
	if len(r.List) > 65535 {
		return nil, errors.New("累计记录数超过标准返回类型 uint16 上限")
	}
	out := &protocol.TradeResp{Count: uint16(len(r.List))}
	for _, k := range r.List {
		if k.Standard == nil {
			return nil, errors.New("缺少标准分笔原始数据")
		}
		out.List = append(out.List, k.Standard)
	}
	return out, nil
}
