package protocol

import (
	"fmt"
	"strings"
	"time"
)

type QuotesResp []*Quote

func (qs QuotesResp) String() string {
	ls := []string(nil)
	for _, v := range qs {
		ls = append(ls, v.String())
	}
	return strings.Join(ls, "\n")
}

type Quote struct {
	Exchange   Exchange // 市场
	Code       string   // 股票代码 6个ascii字符串
	Active1    uint16   // 活跃度
	Kline      *Kline   //k线,这里的时间取得是当前时间,昨收盘好像不太对
	ServerTime string   // 时间
	Reserved0  int      `json:"ReversedBytes0"` // 保留(时间 ServerTime)
	Reserved1  int      `json:"ReversedBytes1"` // 保留 这个等于 负的收盘价格?
	Intuition  int      // 现量（东财的盘口-现量）现在成交量
	InsideDish int      // 内盘（东财的盘口-内盘）（和东财对不上）
	OuterDisc  int      // 外盘（东财的盘口-外盘）（和东财对不上）

	Reserved2 int         `json:"ReversedBytes2"` // 保留，未知
	Reserved3 int         `json:"ReversedBytes3"` // 保留，未知,基金的昨收净值?
	BuyLevel  PriceLevels // 5档买盘(买1-5)
	SellLevel PriceLevels // 5档卖盘(卖1-5)

	Reserved4 uint16  `json:"ReversedBytes4"` // 保留，未知
	Reserved5 int     `json:"ReversedBytes5"` // 保留，未知
	Reserved6 int     `json:"ReversedBytes6"` // 保留，未知
	Reserved7 int     `json:"ReversedBytes7"` // 保留，未知
	Reserved8 int     `json:"ReversedBytes8"` // 保留，未知
	Reserved9 uint16  `json:"ReversedBytes9"` // 保留，未知
	Rate      float64 // 涨速，好像都是0
	Active2   uint16  // 活跃度
}

func (q *Quote) String() string {
	return fmt.Sprintf(`%s%s
%s
现量：%s,  内盘：%s, 外盘：%s
%s%s
`,
		q.Exchange.String(), q.Code, q.Kline,
		IntUnitString(q.Intuition),
		IntUnitString(q.InsideDish), IntUnitString(q.OuterDisc),
		q.SellLevel.String(), q.BuyLevel.String(),
	)
}

type quote struct{}

func (quote) Frame(codes ...string) (*Frame, error) {
	f := &Frame{
		Control: Control01,
		Type:    TypeQuote,
		Data:    []byte{0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	payload := Bytes(uint16(len(codes)))
	for _, v := range codes {
		exchange, code, err := DecodeCode(v)
		if err != nil {
			return nil, err
		}

		payload = append(payload, exchange.Uint8())
		payload = append(payload, code...)
	}
	f.Data = append(f.Data, payload...)

	return f, nil
}

/*
Decode
0136
0200  数量
00  交易所
303030303031 股票代码
320b 活跃度？
b212 昨天收盘价1186
4c
56
10
59
87e6d10cf212b78fa801ae01293dc54e8bd740acb8670086ca1e0001af36ba0c4102b467b6054203a68a0184094304891992114405862685108d0100000000e8ff320b

01 深圳交易所
363030303038 股票代码
5909
8005
46
45
02
46
8defd10c 服务时间
c005bed2668e05be15804d8ba12cb3b13a0083c3034100badc029d014201bc990384f70443029da503b7af074403a6e501b9db044504a6e2028dd5048d050000000000005909
*/
// Decode 保留原接口；畸形数据返回 nil。需要错误详情时使用 DecodeChecked。
func (q quote) Decode(bs []byte) QuotesResp {
	resp, _ := q.DecodeChecked(bs)
	return resp
}

func (quote) DecodeChecked(bs []byte) (QuotesResp, error) {
	d := wireDecoder{bs: bs}
	d.take(2) // 响应前缀
	number := d.uint16()
	if d.err != nil {
		return nil, d.err
	}
	resp := QuotesResp{}
	for i := uint16(0); i < number; i++ {
		prefix := d.take(9)
		if d.err != nil {
			return nil, fmt.Errorf("报价记录 %d: %w", i, d.err)
		}
		sec := &Quote{
			Exchange: Exchange(prefix[0]),
			Code:     string(GBKToUTF8(prefix[1:7])),
			Active1:  Uint16(prefix[7:9]),
		}
		closePrice := Price(d.number()) * 10
		sec.Kline = &Kline{
			Time: time.Now(), Close: closePrice,
			Last: closePrice + Price(d.number())*10,
			Open: closePrice + Price(d.number())*10,
			High: closePrice + Price(d.number())*10,
			Low:  closePrice + Price(d.number())*10,
		}
		sec.Reserved0 = d.number()
		sec.ServerTime = fmt.Sprintf("%d", sec.Reserved0)
		sec.Reserved1 = d.number()
		sec.Kline.Volume = int64(d.number())
		sec.Intuition = d.number()
		sec.Kline.Amount = Yuan(float32Value(d.uint32()))
		sec.InsideDish = d.number()
		sec.OuterDisc = d.number()
		sec.Reserved2 = d.number()
		sec.Reserved3 = d.number()
		for level := 0; level < 5; level++ {
			buyPrice := closePrice + Price(d.number())*10
			sellPrice := closePrice + Price(d.number())*10
			sec.BuyLevel[level] = PriceLevel{Buy: true, Price: buyPrice, Number: d.number()}
			sec.SellLevel[level] = PriceLevel{Price: sellPrice, Number: d.number()}
		}
		sec.Reserved4 = d.uint16()
		sec.Reserved5 = d.number()
		sec.Reserved6 = d.number()
		sec.Reserved7 = d.number()
		sec.Reserved8 = d.number()
		sec.Reserved9 = d.uint16()
		sec.Rate = float64(sec.Reserved9) / 100
		sec.Active2 = d.uint16()
		if d.err != nil {
			return nil, fmt.Errorf("报价记录 %d: %w", i, d.err)
		}
		resp = append(resp, sec)
	}
	return resp, nil
}
