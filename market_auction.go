package tdx

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func (c *MarketClient) GetKlineMinute241Until(code string, stop func(Kline) bool) (*KlineResp, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	if info.Protocol != protocol.ProtocolStandard || (!protocol.IsStock(standardCode(i)) && !protocol.IsETF(standardCode(i))) {
		return nil, fmt.Errorf("241分钟线仅适用于标准股票/ETF；其他品种使用 GetKlineMinuteUntil")
	}
	resp, err := c.GetKlineMinuteUntil(code, stop)
	if err != nil {
		return nil, err
	}
	if len(resp.List) == 0 {
		return resp, nil
	}
	ks := []Kline{}
	for _, record := range resp.List {
		v := record.Standard
		if v.Time.Format(time.TimeOnly) == "09:31:00" {
			var tr *TradeResp
			if v.Time.Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
				tr, err = c.GetTradeAll(code)
			} else {
				tr, err = c.GetHistoryTradeDay(v.Time.Format("20060102"), code)
			}
			if err != nil {
				return nil, err
			}
			_925 := new(protocol.Trade)
			if len(tr.List) > 0 && tr.List[0].Standard.Time.Format(time.TimeOnly) < "09:30:00" {
				_925 = tr.List[0].Standard
			}
			ks = append(ks, standardKline(&protocol.Kline{
				Last:   v.Last,
				Open:   _925.Price,
				High:   _925.Price,
				Low:    _925.Price,
				Close:  _925.Price,
				Order:  _925.Number,
				Volume: int64(_925.Volume),
				Amount: _925.Amount(),
				Time:   time.Date(v.Time.Year(), v.Time.Month(), v.Time.Day(), 9, 30, 0, 0, v.Time.Location()),
			}))
			v.Last = _925.Price
			v.Volume -= int64(_925.Volume)
			if v.Volume < 0 {
				v.Volume = 0
			}
			v.Amount -= _925.Amount()
			if v.Amount < 0 {
				v.Amount = 0
			}
			v.Order -= _925.Number
			if v.Order < 0 {
				v.Order = 0
			}
		}
		ks = append(ks, standardKline(v))
	}
	resp.List = ks
	resp.Count = len(ks)
	return resp, nil
}
