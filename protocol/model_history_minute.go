package protocol

import (
	"errors"

	"github.com/injoyai/conv"
	"io"
)

type historyMinute struct{}

// MinuteCache carries the market session needed to derive clocks for layouts
// whose records have no timestamp of their own.
type MinuteCache struct {
	Session TradingSession
}

func (historyMinute) Frame(date, code string) (*Frame, error) {
	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}
	dataBs := Bytes(conv.Uint32(date))
	dataBs = append(dataBs, exchange.Uint8())
	dataBs = append(dataBs, []byte(number)...)
	return &Frame{
		Control: Control01,
		Type:    TypeHistoryMinute,
		Data:    dataBs,
	}, nil
}

func (historyMinute) Decode(bs []byte, cache ...MinuteCache) (*MinuteResp, error) {

	if len(bs) < 6 {
		return nil, errors.New("数据长度不足")
	}

	session := DomesticSession
	if len(cache) > 0 {
		session = cache[0].Session
	}

	resp := &MinuteResp{
		Count: Uint16(bs[:2]),
	}

	//2-4字节是啥?
	d := wireDecoder{bs: bs[6:]}
	if int(resp.Count) > len(d.bs)/3 {
		return nil, io.ErrUnexpectedEOF
	}
	if resp.Count > 0 {
		resp.List = make([]PriceNumber, 0, resp.Count)
	}

	var lastPrice int64
	for i := uint16(0); i < resp.Count; i++ {
		price := int64(d.number())
		d.number() // 未知字段
		lastPrice += price
		number := d.number()
		if d.err != nil {
			return nil, d.err
		}

		clock, err := session.HourMinute(int(i))
		if err != nil {
			return nil, err
		}
		resp.List = append(resp.List, PriceNumber{
			Time:   clock,
			Price:  float64(lastPrice) / 100,
			Number: number,
		})
	}
	return resp, nil
}
