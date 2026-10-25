package protocol

import (
	"encoding/binary"
	"io"
)

// 扩展行情协议(TdxExHq)——期货/期权/港股/外汇/全球指数。
// 移植自 pytdx exhq 解析器。与标准行情共用传输层(0xB1CB7400 响应帧+zlib),
// 区别仅在请求帧头前缀为 0x01(标准为 0x0C),故复用 Frame{Prefix:0x01,...}。
const (
	TypeExSetup      uint16 = 0x2454 // 扩展行情握手(setup)
	TypeExMarkets    uint16 = 0x23F4 // 市场代码表
	TypeExCount      uint16 = 0x23F0 // 品种数量
	TypeExInstrument uint16 = 0x23F5 // 品种(代码)列表
	TypeExQuote      uint16 = 0x23FA // 单品种五档行情
	TypeExQuoteList  uint16 = 0x2400 // 批量行情列表
	TypeExBars       uint16 = 0x23FF // K线
	TypeExMinute     uint16 = 0x240B // 当日分时
	TypeExHistMinute uint16 = 0x240C // 历史分时
	TypeExTrade      uint16 = 0x23FC // 分笔成交
	TypeExHistTrade  uint16 = 0x2406 // 历史分笔成交
	TypeExBarsRange  uint16 = 0x240D // 历史K线区间
)

const exPrefix byte = 0x01

// ---- 结果结构 ----

// ExMarket 扩展市场。
type ExMarket struct {
	Market    uint16 `json:"market"`
	Category  uint8  `json:"category"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
}

// ExInstrument 扩展品种(代码表项)。
type ExInstrument struct {
	Category uint8  `json:"category"`
	Market   uint8  `json:"market"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Desc     string `json:"desc"`
}

// ExQuote 扩展单品种五档行情。
type ExQuote struct {
	Market    uint8      `json:"market"`
	Code      string     `json:"code"`
	PreClose  float64    `json:"preClose"`  // 昨收/昨结，原始浮点值，原币种/报价单位
	Open      float64    `json:"open"`      // 开盘价，原始浮点值，原币种/报价单位
	High      float64    `json:"high"`      // 最高价，原始浮点值，原币种/报价单位
	Low       float64    `json:"low"`       // 最低价，原始浮点值，原币种/报价单位
	Price     float64    `json:"price"`     // 最新价，原币种/报价单位
	KaiCang   uint32     `json:"kaiCang"`   // 开仓
	ZongLiang uint32     `json:"zongLiang"` // 总量
	XianLiang uint32     `json:"xianLiang"` // 现量
	NeiPan    uint32     `json:"neiPan"`    // 内盘
	WaiPan    uint32     `json:"waiPan"`    // 外盘
	ChiCang   uint32     `json:"chiCang"`   // 持仓
	Bid       [5]float64 `json:"bid"`       // 买一至买五价格，原币种/报价单位
	BidVol    [5]uint32  `json:"bidVol"`
	Ask       [5]float64 `json:"ask"` // 卖一至卖五价格，原币种/报价单位
	AskVol    [5]uint32  `json:"askVol"`
}

// ExKline 扩展K线一根。
type ExKline struct {
	Datetime string  `json:"datetime"`
	Open     float64 `json:"open"`     // 开盘价，原始浮点值，原币种/报价单位
	High     float64 `json:"high"`     // 最高价，原始浮点值，原币种/报价单位
	Low      float64 `json:"low"`      // 最低价，原始浮点值，原币种/报价单位
	Close    float64 `json:"close"`    // 收盘价/该周期最新价，原始浮点值，原币种/报价单位
	Position uint32  `json:"position"` // 持仓
	Trade    uint32  `json:"trade"`    // 成交量
	Price    float64 `json:"price"`    // 结算价，原币种/报价单位
	Amount   float64 `json:"amount"`   // position 字节按 float32 重解释的原始值，金额单位/语义未确认
}

// ExMinuteTick 扩展分时一笔。
type ExMinuteTick struct {
	Hour         int     `json:"hour"`
	Minute       int     `json:"minute"`
	Price        float64 `json:"price"`    // 分时价格，原币种/报价单位
	AvgPrice     float64 `json:"avgPrice"` // 分时均价，原币种/报价单位
	Volume       uint32  `json:"volume"`
	OpenInterest uint32  `json:"openInterest"`
}

// ExTradeTick 扩展分笔成交一笔。
type ExTradeTick struct {
	Hour       int    `json:"hour"`
	Minute     int    `json:"minute"`
	Second     int    `json:"second"`
	Price      uint32 `json:"price"` // 原始整数价格，尺度未确认；不可直接当作元值或自行除以100/1000
	Volume     uint32 `json:"volume"`
	ZengCang   int32  `json:"zengCang"` // 增仓
	Nature     uint16 `json:"nature"`
	NatureName string `json:"natureName"`
	Direction  int    `json:"direction"` // 1买 -1卖 0中性
}

// ExRangeKline 扩展历史K线区间一根。
type ExRangeKline struct {
	Datetime        string  `json:"datetime"`
	Open            float64 `json:"open"`  // 开盘价，原始浮点值，原币种/报价单位
	High            float64 `json:"high"`  // 最高价，原始浮点值，原币种/报价单位
	Low             float64 `json:"low"`   // 最低价，原始浮点值，原币种/报价单位
	Close           float64 `json:"close"` // 收盘价/该周期最新价，原始浮点值，原币种/报价单位
	Position        uint32  `json:"position"`
	Trade           uint32  `json:"trade"`
	SettlementPrice float64 `json:"settlementPrice"` // 结算价，原币种/报价单位
}

// QuoteListItem 批量行情列表项(期货 category=3 / 港股 category=2)。
type QuoteListItem struct {
	Market    uint8      `json:"market"`
	Code      string     `json:"code"`
	PreClose  float64    `json:"preClose"`  // 昨收/昨结，原始浮点值，原币种/报价单位
	Open      float64    `json:"open"`      // 开盘价，原始浮点值，原币种/报价单位
	High      float64    `json:"high"`      // 最高价，原始浮点值，原币种/报价单位
	Low       float64    `json:"low"`       // 最低价，原始浮点值，原币种/报价单位
	Price     float64    `json:"price"`     // 现价/卖出价，原币种/报价单位
	ZongLiang uint32     `json:"zongLiang"` // 总量
	Amount    float64    `json:"amount"`    // 原始总金额字段，单位/统计口径待确认
	Inner     uint32     `json:"inner"`     // 内盘
	Outer     uint32     `json:"outer"`     // 外盘
	ChiCang   uint32     `json:"chiCang"`   // 持仓(期货)
	Bid       [5]float64 `json:"bid"`       // 买一至买五价格，原币种/报价单位
	BidVol    [5]uint32  `json:"bidVol"`
	Ask       [5]float64 `json:"ask"` // 卖一至卖五价格，原币种/报价单位
	AskVol    [5]uint32  `json:"askVol"`
}

// ---- 协议单例 ----

type exHq struct{}

// MEx 扩展行情协议单例。
var MEx exHq

// exFrame 构造扩展行情请求帧(前缀 0x01)。
func exFrame(typ uint16, control Control, data []byte) *Frame {
	return &Frame{Prefix: exPrefix, Control: control, Type: typ, Data: data}
}

func putCode(dst []byte, code string) {
	copy(dst, code) // 9 字节,不足补 0
}

// ---- Setup ----

// FrameSetup 握手帧。响应忽略。
func (exHq) FrameSetup() *Frame {
	data := []byte{
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0x1f, 0x32, 0xc6, 0xe5, 0xd5, 0x3d, 0xfb, 0x41,
		0xcc, 0xe1, 0x6d, 0xff, 0xd5, 0xba, 0x3f, 0xb8,
		0xcb, 0xc5, 0x7a, 0x05, 0x4f, 0x77, 0x48, 0xea,
	}
	return exFrame(TypeExSetup, Control01, data)
}

// ---- 市场 ----

// FrameMarkets 市场代码表请求。
func (exHq) FrameMarkets() *Frame { return exFrame(TypeExMarkets, Control01, nil) }

// DecodeMarkets 解析市场代码表。
func (exHq) DecodeMarkets(bs []byte) ([]ExMarket, error) {
	if len(bs) < 2 {
		return nil, io.ErrUnexpectedEOF
	}
	r := &exReader{b: bs, p: 2}
	cnt := int(Uint16(bs[0:2]))
	if cnt > r.remain()/64 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]ExMarket, 0, cnt)
	for i := 0; i < cnt; i++ {
		category := r.u8()
		name := decodeGBKName(r.bytes(32))
		market := r.u8()
		short := decodeGBKName(r.bytes(2))
		r.skip(28) // 26s + 2s
		if category == 0 && market == 0 {
			continue
		}
		out = append(out, ExMarket{Market: uint16(market), Category: category, Name: name, ShortName: short})
	}
	return out, nil
}

// ---- 品种数量 ----

// FrameCount 品种数量请求。
func (exHq) FrameCount() *Frame { return exFrame(TypeExCount, Control01, nil) }

// DecodeCount 解析品种数量。
func (exHq) DecodeCount(bs []byte) (int, error) {
	if len(bs) < 23 {
		return 0, io.ErrUnexpectedEOF
	}
	return int(Uint32(bs[19:23])), nil
}

// ---- 品种列表 ----

// FrameInstrument 品种列表请求(分页)。
func (exHq) FrameInstrument(start uint32, count uint16) *Frame {
	data := make([]byte, 6)
	binary.LittleEndian.PutUint32(data[0:4], start)
	binary.LittleEndian.PutUint16(data[4:6], count)
	return exFrame(TypeExInstrument, Control01, data)
}

// DecodeInstrument 解析品种列表。每项占 64 字节(仅前 40 字节有效)。
func (exHq) DecodeInstrument(bs []byte) ([]ExInstrument, error) {
	if len(bs) < 6 {
		return nil, io.ErrUnexpectedEOF
	}
	count := int(Uint16(bs[4:6]))
	r := &exReader{b: bs, p: 6}
	if count > r.remain()/64 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]ExInstrument, 0, count)
	for i := 0; i < count; i++ {
		base := r.p
		category := r.u8()
		market := r.u8()
		r.skip(3) // unused
		code := decodeGBKName(r.bytes(9))
		name := decodeGBKName(r.bytes(17))
		desc := decodeGBKName(r.bytes(9))
		r.p = base + 64
		out = append(out, ExInstrument{Category: category, Market: market, Code: code, Name: name, Desc: desc})
	}
	return out, nil
}

// ---- 单品种五档 ----

// FrameQuote 单品种五档请求。
func (exHq) FrameQuote(market uint8, code string) *Frame {
	data := make([]byte, 10)
	data[0] = market
	putCode(data[1:10], code)
	return exFrame(TypeExQuote, Control01, data)
}

// DecodeQuote 解析单品种五档。
func (exHq) DecodeQuote(bs []byte) (*ExQuote, error) {
	if len(bs) < 150 {
		return nil, io.ErrUnexpectedEOF
	}
	q := &ExQuote{Market: bs[0], Code: cutAtNull(bs[1:10])}
	r := &exReader{b: bs, p: 14} // 跳过 10(market+code) + 4
	q.PreClose = r.f32()
	q.Open = r.f32()
	q.High = r.f32()
	q.Low = r.f32()
	q.Price = r.f32()
	q.KaiCang = r.u32()
	r.skip(4)
	q.ZongLiang = r.u32()
	q.XianLiang = r.u32()
	r.skip(4)
	q.NeiPan = r.u32()
	q.WaiPan = r.u32()
	r.skip(4)
	q.ChiCang = r.u32()
	for i := 0; i < 5; i++ {
		q.Bid[i] = r.f32()
	}
	for i := 0; i < 5; i++ {
		q.BidVol[i] = r.u32()
	}
	for i := 0; i < 5; i++ {
		q.Ask[i] = r.f32()
	}
	for i := 0; i < 5; i++ {
		q.AskVol[i] = r.u32()
	}
	return q, nil
}

// ---- K线 ----

// ExBarsCache K线解析所需上下文(category 决定时间编码)。
type ExBarsCache struct{ Category uint8 }

// FrameBars K线请求。
func (exHq) FrameBars(category uint8, market uint8, code string, start, count uint16) *Frame {
	data := make([]byte, 20)
	data[0] = market
	putCode(data[1:10], code)
	binary.LittleEndian.PutUint16(data[10:12], uint16(category))
	binary.LittleEndian.PutUint16(data[12:14], 1) // 疑似复权标志
	binary.LittleEndian.PutUint32(data[14:18], uint32(start))
	binary.LittleEndian.PutUint16(data[18:20], count)
	return exFrame(TypeExBars, Control01, data)
}

// DecodeBars 解析K线。
func (exHq) DecodeBars(bs []byte, c ExBarsCache) ([]ExKline, error) {
	if len(bs) < 20 {
		return nil, io.ErrUnexpectedEOF
	}
	r := &exReader{b: bs, p: 18}
	cnt := int(r.u16())
	if cnt > r.remain()/32 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]ExKline, 0, cnt)
	for i := 0; i < cnt; i++ {
		var dt [4]byte
		copy(dt[:], r.bytes(4))
		t := GetTime(dt, c.Category)
		base := r.p
		open := r.f32()
		high := r.f32()
		low := r.f32()
		cls := r.f32()
		amount := float64(Float32(bs[base+16 : base+20])) // 与 position 字节重叠
		position := r.u32()
		trade := r.u32()
		price := r.f32()
		out = append(out, ExKline{
			Datetime: KlineTime(t, c.Category),
			Open:     open, High: high, Low: low, Close: cls,
			Position: position, Trade: trade, Price: price, Amount: amount,
		})
	}
	return out, nil
}

// ---- 分时 ----

// FrameMinute 当日分时请求。
func (exHq) FrameMinute(market uint8, code string) *Frame {
	data := make([]byte, 10)
	data[0] = market
	putCode(data[1:10], code)
	return exFrame(TypeExMinute, Control01, data)
}

// DecodeMinute 解析当日分时(头部 12 字节)。
func (exHq) DecodeMinute(bs []byte) ([]ExMinuteTick, error) {
	return decodeExMinute(bs, 12)
}

// FrameHistMinute 历史分时请求。
func (exHq) FrameHistMinute(market uint8, code string, date uint32) *Frame {
	data := make([]byte, 14)
	binary.LittleEndian.PutUint32(data[0:4], date)
	data[4] = market
	putCode(data[5:14], code)
	return exFrame(TypeExHistMinute, Control01, data)
}

// DecodeHistMinute 解析历史分时(头部 20 字节)。
func (exHq) DecodeHistMinute(bs []byte) ([]ExMinuteTick, error) {
	return decodeExMinute(bs, 20)
}

func decodeExMinute(bs []byte, headerLen int) ([]ExMinuteTick, error) {
	if len(bs) < headerLen {
		return nil, io.ErrUnexpectedEOF
	}
	num := int(Uint16(bs[headerLen-2 : headerLen]))
	r := &exReader{b: bs, p: headerLen}
	if num > r.remain()/18 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]ExMinuteTick, 0, num)
	for i := 0; i < num; i++ {
		raw := int(r.u16())
		price := r.f32()
		avg := r.f32()
		vol := r.u32()
		amt := r.u32()
		out = append(out, ExMinuteTick{
			Hour: raw / 60, Minute: raw % 60,
			Price: price, AvgPrice: avg, Volume: vol, OpenInterest: amt,
		})
	}
	return out, nil
}

// ---- 分笔成交 ----

// ExTradeCache 分笔解析上下文(market 决定港股 B/S 判定)。
type ExTradeCache struct{ Market uint8 }

// FrameTrade 当日分笔请求。
func (exHq) FrameTrade(market uint8, code string, start, count uint16) *Frame {
	data := make([]byte, 16)
	data[0] = market
	putCode(data[1:10], code)
	binary.LittleEndian.PutUint32(data[10:14], uint32(int32(start)))
	binary.LittleEndian.PutUint16(data[14:16], count)
	return exFrame(TypeExTrade, Control01, data)
}

// DecodeTrade 解析当日分笔(头部 16 字节)。
func (exHq) DecodeTrade(bs []byte, c ExTradeCache) ([]ExTradeTick, error) {
	return decodeExTrade(bs, c.Market)
}

// FrameHistTrade 历史分笔请求。
func (exHq) FrameHistTrade(market uint8, code string, date uint32, start, count uint16) *Frame {
	data := make([]byte, 20)
	binary.LittleEndian.PutUint32(data[0:4], date)
	data[4] = market
	putCode(data[5:14], code)
	binary.LittleEndian.PutUint32(data[14:18], uint32(int32(start)))
	binary.LittleEndian.PutUint16(data[18:20], count)
	return exFrame(TypeExHistTrade, Control01, data)
}

// DecodeHistTrade 解析历史分笔(头部 16 字节,与当日同构)。
func (exHq) DecodeHistTrade(bs []byte, c ExTradeCache) ([]ExTradeTick, error) {
	return decodeExTrade(bs, c.Market)
}

func decodeExTrade(bs []byte, market uint8) ([]ExTradeTick, error) {
	if len(bs) < 16 {
		return nil, io.ErrUnexpectedEOF
	}
	num := int(Uint16(bs[14:16]))
	r := &exReader{b: bs, p: 16}
	if num > r.remain()/16 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]ExTradeTick, 0, num)
	for i := 0; i < num; i++ {
		raw := int(r.u16())
		price := r.u32()
		volume := r.u32()
		zengcang := r.i32()
		direction := r.u16()
		second := int(direction) % 10000
		if second > 59 {
			second = 0
		}
		dir, name := exTradeNature(market, direction, volume, zengcang)
		out = append(out, ExTradeTick{
			Hour: raw / 60, Minute: raw % 60, Second: second,
			Price: price, Volume: volume, ZengCang: zengcang,
			Nature: direction, NatureName: name, Direction: dir,
		})
	}
	return out, nil
}

// exTradeNature 还原买卖方向与开平性质,移植自 pytdx。
func exTradeNature(market uint8, direction uint16, volume uint32, zengcang int32) (int, string) {
	if market == 31 || market == 48 { // 港股
		switch direction {
		case 0:
			return 1, "B"
		case 256:
			return -1, "S"
		default:
			return 0, ""
		}
	}
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

// ---- 历史K线区间 ----

// FrameBarsRange 历史K线区间请求。
func (exHq) FrameBarsRange(market uint8, code string, date, date2 uint32) *Frame {
	data := make([]byte, 20)
	data[0] = market
	putCode(data[1:10], code)
	data[10] = 0x07
	data[11] = 0x00
	binary.LittleEndian.PutUint32(data[12:16], date)
	binary.LittleEndian.PutUint32(data[16:20], date2)
	return exFrame(TypeExBarsRange, Control01, data)
}

// DecodeBarsRange 解析历史K线区间。
func (exHq) DecodeBarsRange(bs []byte) ([]ExRangeKline, error) {
	if len(bs) < 14 {
		return nil, io.ErrUnexpectedEOF
	}
	r := &exReader{b: bs, p: 12}
	cnt := int(r.u16())
	if cnt > r.remain()/32 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]ExRangeKline, 0, cnt)
	for i := 0; i < cnt; i++ {
		d1 := int(r.u16())
		d2 := int(r.u16())
		open := r.f32()
		high := r.f32()
		low := r.f32()
		cls := r.f32()
		position := r.u32()
		trade := r.u32()
		settle := r.f32()
		year := d1/2048 + 2004
		month := (d1 % 2048) / 100
		day := (d1 % 2048) % 100
		hour := d2 / 60
		minute := d2 % 60
		out = append(out, ExRangeKline{
			Datetime: fmtDatetime(year, month, day, hour, minute),
			Open:     open, High: high, Low: low, Close: cls,
			Position: position, Trade: trade, SettlementPrice: settle,
		})
	}
	return out, nil
}

// ---- 批量行情列表 ----

// ExQuoteListCache 批量行情解析上下文(category 2=港股 3=期货)。
type ExQuoteListCache struct{ Category uint8 }

// FrameQuoteList 批量行情列表请求。
func (exHq) FrameQuoteList(market uint8, start, count uint16) *Frame {
	data := make([]byte, 9)
	data[0] = market
	binary.LittleEndian.PutUint16(data[1:3], 0)
	binary.LittleEndian.PutUint16(data[3:5], start)
	binary.LittleEndian.PutUint16(data[5:7], count)
	binary.LittleEndian.PutUint16(data[7:9], 1)
	return exFrame(TypeExQuoteList, 0x02, data)
}

// DecodeQuoteList 解析批量行情列表(仅支持期货 cat3 / 港股 cat2)。每项 300 字节。
func (exHq) DecodeQuoteList(bs []byte, c ExQuoteListCache) ([]QuoteListItem, error) {
	if len(bs) < 2 {
		return nil, io.ErrUnexpectedEOF
	}
	num := int(Uint16(bs[0:2]))
	if num == 0 || (c.Category != 2 && c.Category != 3) {
		return nil, nil
	}
	r := &exReader{b: bs, p: 2}
	if num > r.remain()/300 {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]QuoteListItem, 0, num)
	for i := 0; i < num; i++ {
		market := r.u8()
		code := decodeGBKName(r.bytes(9))
		base := r.p // 数据块 140 字节有效,整体步进 290
		it := QuoteListItem{Market: market, Code: code}
		if c.Category == 2 {
			parseExHK(bs, base, &it)
		} else {
			parseExFutures(bs, base, &it)
		}
		r.p = base + 290
		out = append(out, it)
	}
	return out, nil
}

// parseExHK 港股批量行情块(140 字节)。
func parseExHK(bs []byte, p int, it *QuoteListItem) {
	if p+140 > len(bs) {
		return
	}
	rd := &exReader{b: bs, p: p}
	rd.skip(4) // HuoYueDu
	it.PreClose = rd.f32()
	it.Open = rd.f32()
	it.High = rd.f32()
	it.Low = rd.f32()
	it.Price = rd.f32()
	rd.skip(4) // 0
	rd.skip(4) // MaiRuJia(参考)
	it.ZongLiang = rd.u32()
	rd.skip(4) // XianLiang
	it.Amount = rd.f32()
	rd.skip(8) // 2 未知
	it.Inner = rd.u32()
	it.Outer = rd.u32()
	for i := 0; i < 5; i++ {
		it.Bid[i] = rd.f32()
	}
	for i := 0; i < 5; i++ {
		it.BidVol[i] = rd.u32()
	}
	for i := 0; i < 5; i++ {
		it.Ask[i] = rd.f32()
	}
	for i := 0; i < 5; i++ {
		it.AskVol[i] = rd.u32()
	}
}

// parseExFutures 期货批量行情块(140 字节)。
func parseExFutures(bs []byte, p int, it *QuoteListItem) {
	if p+140 > len(bs) {
		return
	}
	rd := &exReader{b: bs, p: p}
	rd.skip(4) // BiShu
	it.PreClose = rd.f32()
	it.Open = rd.f32()
	it.High = rd.f32()
	it.Low = rd.f32()
	it.Price = rd.f32() // MaiChu(卖出)
	rd.skip(4)          // KaiCang
	rd.skip(4)          // 未知
	it.ZongLiang = rd.u32()
	rd.skip(4) // XianLiang
	it.Amount = rd.f32()
	it.Inner = rd.u32()   // NeiPan
	it.Outer = rd.u32()   // WaiPan
	rd.skip(4)            // 未知 float
	it.ChiCang = rd.u32() // ChiCangLiang
	it.Ask[0] = rd.f32()  // MaiRuJia
	rd.skip(16)           // 4 未知
	it.AskVol[0] = rd.u32()
}

// fmtDatetime 格式化 "YYYY-MM-DD HH:MM"。
func fmtDatetime(y, mo, d, h, mi int) string {
	buf := []byte("0000-00-00 00:00")
	put2 := func(off, v int) {
		buf[off] = byte('0' + (v/10)%10)
		buf[off+1] = byte('0' + v%10)
	}
	buf[0] = byte('0' + (y/1000)%10)
	buf[1] = byte('0' + (y/100)%10)
	buf[2] = byte('0' + (y/10)%10)
	buf[3] = byte('0' + y%10)
	put2(5, mo)
	put2(8, d)
	put2(11, h)
	put2(14, mi)
	return string(buf)
}

// exReader 小端游标读取器。
type exReader struct {
	b []byte
	p int
}

func (r *exReader) u8() uint8 {
	v := r.b[r.p]
	r.p++
	return v
}
func (r *exReader) u16() uint16 {
	v := Uint16(r.b[r.p : r.p+2])
	r.p += 2
	return v
}
func (r *exReader) u32() uint32 {
	v := Uint32(r.b[r.p : r.p+4])
	r.p += 4
	return v
}
func (r *exReader) i32() int32 { return int32(r.u32()) }
func (r *exReader) f32() float64 {
	v := Float32(r.b[r.p : r.p+4])
	r.p += 4
	return float64(v)
}
func (r *exReader) bytes(n int) []byte {
	v := r.b[r.p : r.p+n]
	r.p += n
	return v
}
func (r *exReader) skip(n int)  { r.p += n }
func (r *exReader) remain() int { return len(r.b) - r.p }
