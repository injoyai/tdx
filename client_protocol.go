package tdx

import (
	"errors"
	"fmt"
	"sync"

	"github.com/injoyai/tdx/v2/protocol"
)

// StandardClient 实现标准行情业务，复用 Conn 的收发、超时和重连管理。
// 使用 NewStandardClient 包装 Dial* 返回的标准连接；同一连接只创建一个业务实现。
type StandardClient struct {
	*Conn
	quoteMu    sync.Mutex
	quoteCodes *CodesBase
}

// ExtendedClient 实现扩展行情业务；财务、F10 等标准专用查询返回不支持错误。
// 使用 NewExtendedClient 包装 DialExHq* 返回的连接，不会建立标准连接。
type ExtendedClient struct {
	*Conn
}

// protocolClient 的未导出入口供共用日期校验和分页调用，避免每页重复检查历史日线。
type protocolClient interface {
	Client
	minutePage(date, code string) (*MinuteResp, error)
	tradePage(date, code string, start, count uint16) (*TradeResp, error)
}

// NewStandardClient 复用已建立的标准连接；关闭返回对象也会关闭该连接。
func NewStandardClient(conn *Conn) (*StandardClient, error) {
	if conn == nil || conn.extended {
		return nil, errors.New("需要非空标准行情连接")
	}
	return conn.protocolClient().(*StandardClient), nil
}

// NewExtendedClient 复用已建立的扩展连接；关闭返回对象也会关闭该连接。
func NewExtendedClient(conn *Conn) (*ExtendedClient, error) {
	if conn == nil || !conn.extended {
		return nil, errors.New("需要非空扩展行情连接")
	}
	return conn.protocolClient().(*ExtendedClient), nil
}

func (c *Conn) protocolInstrument(code string, extended bool) (protocol.Instrument, protocol.MarketInfo, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return i, info, err
	}
	if c.extended != extended || (info.Protocol == protocol.ProtocolExtended) != extended {
		return i, info, errors.New("证券与当前连接协议不匹配；跨市场查询请使用 NewClient")
	}
	return i, info, nil
}

func (c *Conn) protocolExchange(ex protocol.Exchange, extended bool) error {
	m, err := (protocol.Instrument{Exchange: ex}).Market()
	if err != nil {
		return err
	}
	info, _ := m.Info()
	if c.extended != extended || (info.Protocol == protocol.ProtocolExtended) != extended {
		return errors.New("证券与当前连接协议不匹配；跨市场查询请使用 NewClient")
	}
	return nil
}

func klineRequest(code string, category uint8, count uint16) (*KlineResp, protocol.MarketInfo, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, info, err
	}
	if category > protocol.TypeKlineYear || count == 0 || count > 800 {
		return nil, info, errors.New("K线周期必须为0..11，单次数量必须为1..800")
	}
	unit := VolumeUnknown
	switch category {
	case protocol.TypeKlineDay, protocol.TypeKlineDay2:
		category, unit = info.Day.Category, info.Day.VolumeUnit
	case protocol.TypeKlineMinute, protocol.TypeKlineMinute2:
		category, unit = info.Minute.Category, info.Minute.VolumeUnit
	default:
		if info.Protocol == protocol.ProtocolStandard {
			unit = info.Day.VolumeUnit
		}
	}
	if unit == protocol.VolumeDefault || protocol.IsConvertibleBond(standardCode(i)) {
		unit = VolumeUnknown
	}
	return &KlineResp{Instrument: i, Category: category, VolumeUnit: unit}, info, nil
}

// GetKline 请求标准 K 线，保留整数价格及指数特有字段。
func (c *StandardClient) GetKline(category uint8, code string, start, count uint16) (*KlineResp, error) {
	out, info, err := klineRequest(code, category, count)
	if err != nil {
		return nil, err
	}
	if _, _, err := c.protocolInstrument(code, false); err != nil {
		return nil, err
	}
	r, err := c.getStandardKline(out.Category, standardCode(out.Instrument), start, count, info.Kind)
	if err != nil {
		return nil, err
	}
	for _, k := range r.List {
		out.List = append(out.List, standardKline(k))
	}
	if len(out.List) > int(count) {
		return nil, errors.New("K线响应数量超过请求数量")
	}
	out.Count = len(out.List)
	return out, nil
}

// GetKline 请求扩展 K 线，保留持仓、结算价等扩展字段。
func (c *ExtendedClient) GetKline(category uint8, code string, start, count uint16) (*KlineResp, error) {
	out, _, err := klineRequest(code, category, count)
	if err != nil {
		return nil, err
	}
	i, _, err := c.protocolInstrument(code, true)
	if err != nil {
		return nil, err
	}
	bars, err := c.getExtendedKline(out.Category, i.Exchange.Uint8(), i.Code, start, count)
	if err != nil {
		return nil, err
	}
	for j := range bars {
		k := &bars[j]
		out.List = append(out.List, Kline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: int64(k.Trade), Extended: k})
	}
	if len(out.List) > int(count) {
		return nil, errors.New("K线响应数量超过请求数量")
	}
	out.Count = len(out.List)
	return out, nil
}

// GetQuote 获取标准五档报价，保持输入顺序及重复代码。
func (c *StandardClient) GetQuote(codes ...string) ([]Quote, error) {
	instruments := make([]protocol.Instrument, len(codes))
	standard := make([]string, len(codes))
	for j, code := range codes {
		i, _, err := c.protocolInstrument(code, false)
		if err != nil {
			return nil, err
		}
		instruments[j], standard[j] = i, standardCode(i)
	}
	if len(standard) > (65535-12)/7 {
		return nil, errors.New("标准报价请求超过协议 uint16 帧长上限")
	}
	out := make([]Quote, len(codes))
	if len(codes) == 0 {
		return out, nil
	}
	quotes, err := c.standardQuotes(standard)
	if err != nil {
		return nil, err
	}
	for j, q := range quotes {
		i := instruments[j]
		if q.Exchange != i.Exchange || q.Code != i.Code {
			return nil, fmt.Errorf("标准行情响应品种与请求不一致: %s", i)
		}
		k := q.Kline
		u := Quote{Instrument: i, PreClose: k.Last.Float64(), Open: k.Open.Float64(), High: k.High.Float64(), Low: k.Low.Float64(), Price: k.Close.Float64(), Volume: k.Volume, VolumeUnit: VolumeUnknown, Standard: q}
		for n := 0; n < 5; n++ {
			u.Bid[n], u.BidVolume[n] = q.BuyLevel[n].Price.Float64(), int64(q.BuyLevel[n].Number)
			u.Ask[n], u.AskVolume[n] = q.SellLevel[n].Price.Float64(), int64(q.SellLevel[n].Number)
		}
		out[j] = u
	}
	return out, nil
}

// GetQuote 逐品种获取扩展五档报价，任一请求失败不返回部分结果。
func (c *ExtendedClient) GetQuote(codes ...string) ([]Quote, error) {
	instruments := make([]protocol.Instrument, len(codes))
	for j, code := range codes {
		i, _, err := c.protocolInstrument(code, true)
		if err != nil {
			return nil, err
		}
		instruments[j] = i
	}
	out := make([]Quote, len(codes))
	for j, i := range instruments {
		q, err := c.getExtendedQuote(i.Exchange.Uint8(), i.Code)
		if err != nil {
			return nil, err
		}
		if q.Market != i.Exchange.Uint8() || q.Code != i.Code {
			return nil, fmt.Errorf("扩展行情响应品种与请求不一致: %s", i)
		}
		u := Quote{Instrument: i, PreClose: q.PreClose, Open: q.Open, High: q.High, Low: q.Low, Price: q.Price, Volume: int64(q.ZongLiang), VolumeUnit: VolumeUnknown, Bid: q.Bid, Ask: q.Ask, Extended: q}
		for n := 0; n < 5; n++ {
			u.BidVolume[n], u.AskVolume[n] = int64(q.BidVol[n]), int64(q.AskVol[n])
		}
		out[j] = u
	}
	return out, nil
}

// standardQuotes 在需要价格精度校准时取元数据，避免全局 DefaultCodes 和额外数据库/连接。
func (c *StandardClient) standardQuotes(codes []string) (protocol.QuotesResp, error) {
	c.quoteMu.Lock()
	defer c.quoteMu.Unlock()
	loaded := map[protocol.Exchange]bool{}
	for _, code := range codes {
		if protocol.IsStock(code) || protocol.IsIndex(code) || c.quoteCodes.Get(code) != nil {
			continue
		}
		ex, _, err := protocol.DecodeCode(code)
		if err != nil {
			return nil, err
		}
		if loaded[ex] {
			continue
		}
		r, err := c.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		var models []*CodeModel
		for _, m := range c.quoteCodes.Iter() {
			if m.Exchange != ex.String() {
				models = append(models, m)
			}
		}
		for _, m := range r.List {
			models = append(models, &CodeModel{Exchange: ex.String(), Code: m.Code, Name: m.Name, Decimal: m.Decimal, Multiple: m.Multiple, LastPrice: m.LastPrice})
		}
		c.quoteCodes.Update(models)
		loaded[ex] = true
	}
	return c.getStandardQuotes(c.quoteCodes, codes...)
}
