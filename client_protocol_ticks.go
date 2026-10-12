package tdx

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol"
)

func (c *StandardClient) minutePage(date, code string) (*MinuteResp, error) {
	i, _, err := c.protocolInstrument(code, false)
	if err != nil {
		return nil, err
	}
	out := &MinuteResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	var r *protocol.MinuteResp
	if date == "" {
		r, err = c.getStandardMinute(standardCode(i))
	} else {
		r, err = c.getStandardHistoryMinute(date, standardCode(i))
	}
	if err != nil {
		return nil, err
	}
	for j := range r.List {
		k := &r.List[j]
		out.List = append(out.List, Minute{Time: k.Time, Price: k.Price.Float64(), Volume: int64(k.Number), Standard: k})
	}

	out.Count = len(out.List)
	return out, nil
}

func (c *ExtendedClient) minutePage(date, code string) (*MinuteResp, error) {
	i, _, err := c.protocolInstrument(code, true)
	if err != nil {
		return nil, err
	}
	out := &MinuteResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	var ticks []protocol.ExMinuteTick
	if date == "" {
		ticks, err = c.getExtendedMinute(i.Exchange.Uint8(), i.Code)
	} else {
		n, _ := parseMarketDate(date)
		ticks, err = c.getExtendedHistoryMinute(i.Exchange.Uint8(), i.Code, n)
	}
	if err != nil {
		return nil, err
	}
	for j := range ticks {
		k := &ticks[j]
		out.List = append(out.List, Minute{Time: fmt.Sprintf("%02d:%02d", k.Hour, k.Minute), Price: k.Price, Volume: int64(k.Volume), Extended: k})
	}
	out.Count = len(out.List)
	return out, nil
}

func (c *StandardClient) tradePage(date, code string, start, count uint16) (*TradeResp, error) {
	i, _, err := c.protocolInstrument(code, false)
	if err != nil {
		return nil, err
	}
	out := &TradeResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	var r *protocol.TradeResp
	if date == "" {
		r, err = c.getStandardTrade(standardCode(i), start, count)
	} else {
		r, err = c.getStandardHistoryTrade(date, standardCode(i), start, count)
	}
	if err != nil {
		return nil, err
	}
	for _, k := range r.List {
		price := k.Price.Float64()
		direction := 0
		if k.IsBuy() {
			direction = 1
		} else if k.IsSell() {
			direction = -1
		}
		out.List = append(out.List, Trade{Date: date, Time: k.Time.Format("15:04:05"), Price: &price, Volume: int64(k.Volume), Direction: direction, Standard: k})
	}

	out.Count = len(out.List)
	return out, nil
}

func (c *ExtendedClient) tradePage(date, code string, start, count uint16) (*TradeResp, error) {
	i, _, err := c.protocolInstrument(code, true)
	if err != nil {
		return nil, err
	}
	out := &TradeResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	var ticks []protocol.ExTradeTick
	if date == "" {
		ticks, err = c.getExtendedTrade(i.Exchange.Uint8(), i.Code, start, count)
	} else {
		n, _ := parseMarketDate(date)
		ticks, err = c.getExtendedHistoryTrade(i.Exchange.Uint8(), i.Code, n, start, count)
	}
	if err != nil {
		return nil, err
	}
	// shortcut: 扩展分笔价格尺度未确认，取得各市场交叉验证样本后填充公共 Price。
	for j := range ticks {
		k := &ticks[j]
		out.List = append(out.List, Trade{Date: date, Time: fmt.Sprintf("%02d:%02d:%02d", k.Hour, k.Minute, k.Second), Volume: int64(k.Volume), Direction: k.Direction, Extended: k})
	}
	out.Count = len(out.List)
	return out, nil
}
