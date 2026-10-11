package protocol

import (
	"errors"

	"github.com/injoyai/conv"
)

type historyTrade struct{}

func (historyTrade) Frame(date, code string, start, count uint16) (*Frame, error) {
	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}
	dataBs := Bytes(conv.Uint32(date)) //req.Time.Format("20060102"))
	dataBs = append(dataBs, exchange.Uint8(), 0x0)
	dataBs = append(dataBs, []byte(number)...)
	dataBs = append(dataBs, Bytes(start)...)
	dataBs = append(dataBs, Bytes(count)...)
	return &Frame{
		Control: Control01,
		Type:    TypeHistoryTrade,
		Data:    dataBs,
	}, nil
}

func (historyTrade) Decode(bs []byte, c TradeCache) (*TradeResp, error) {
	if len(bs) < 6 {
		return nil, errors.New("数据长度不足")
	}

	return decodeTrades(bs[6:], Uint16(bs[:2]), c, true)
}
