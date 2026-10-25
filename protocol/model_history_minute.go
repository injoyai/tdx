package protocol

import (
	"errors"
	"github.com/injoyai/conv"
	"io"
	"time"
)

type historyMinute struct{}

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

func (historyMinute) Decode(bs []byte) (*MinuteResp, error) {

	if len(bs) < 6 {
		return nil, errors.New("数据长度不足")
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
	t := time.Date(0, 0, 0, 9, 30, 0, 0, Location())
	for i := uint16(0); i < resp.Count; i++ {
		price := int64(d.number())
		d.number() // 未知字段
		lastPrice += price
		number := d.number()
		if d.err != nil {
			return nil, d.err
		}

		if i == 120 {
			t = t.Add(time.Minute * 90)
		}
		resp.List = append(resp.List, PriceNumber{
			Time:   t.Add(time.Minute * time.Duration(i+1)).Format("15:04"),
			Price:  float64(lastPrice) / 100,
			Number: number,
		})
	}
	return resp, nil
}
