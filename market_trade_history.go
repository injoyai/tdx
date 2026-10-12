package tdx

import (
	"errors"
	"fmt"
	"time"
)

// TradingCalendar 由调用方提供证券对应市场的交易日；Workday 是沪深日历。
type TradingCalendar interface {
	Range(start, end time.Time, visit func(time.Time) bool)
}

func (c *Conn) GetTradeAll(code string) (*TradeResp, error) {
	return c.queryClient().GetTradeAll(code)
}
func (c *Conn) GetHistoryTradeDay(date, code string) (*TradeResp, error) {
	return c.queryClient().GetHistoryTradeDay(date, code)
}
func (c *Conn) GetHistoryTradeBefore(code string, calendar TradingCalendar, before time.Time) (*TradeResp, error) {
	return c.queryClient().GetHistoryTradeBefore(code, calendar, before)
}
func (c *Conn) GetHistoryTradeFull(code string, calendar TradingCalendar) (*TradeResp, error) {
	return c.queryClient().GetHistoryTradeFull(code, calendar)
}

func (c *MarketClient) GetTradeAll(code string) (*TradeResp, error) {
	return c.tradeAll("", code)
}
func (c *MarketClient) GetHistoryTradeDay(date, code string) (*TradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.tradeAll(date, code)
}

func (c *MarketClient) tradeAll(date, code string) (*TradeResp, error) {
	size := 1800
	if date != "" {
		size = 2000
	}
	var out *TradeResp
	for start := 0; start <= 65535; start += size {
		r, err := c.getTrade(date, code, uint16(start), uint16(size), start == 0)
		if err != nil {
			return nil, err
		}
		if r.Count > size {
			return nil, errors.New("分笔响应数量超过请求数量")
		}
		if out == nil {
			out = &TradeResp{Instrument: r.Instrument, Date: r.Date, VolumeUnit: r.VolumeUnit}
		}
		out.List = append(r.List, out.List...)
		out.Count = len(out.List)
		if r.Count < size {
			return out, nil
		}
	}
	return nil, errors.New("分笔分页偏移超过协议 uint16 上限，历史未完整读取")
}

func (c *MarketClient) GetHistoryTradeFull(code string, calendar TradingCalendar) (*TradeResp, error) {
	return c.GetHistoryTradeBefore(code, calendar, time.Now())
}

// GetHistoryTradeBefore 按给定日历获取最早可用月线所在月份起的整日分笔；before 沿用日历的结束边界。
func (c *MarketClient) GetHistoryTradeBefore(code string, calendar TradingCalendar, before time.Time) (*TradeResp, error) {
	if calendar == nil {
		return nil, errors.New("交易日历不能为空")
	}
	if w, ok := calendar.(*Workday); ok && w == nil {
		return nil, errors.New("交易日历不能为空")
	}
	if before.IsZero() {
		return nil, errors.New("结束时间不能为空")
	}
	r, err := c.GetKlineMonthAll(code)
	if err != nil {
		return nil, err
	}
	out := &TradeResp{Instrument: r.Instrument, VolumeUnit: VolumeUnknown, List: []Trade{}}
	if r.Count == 0 {
		return out, nil
	}
	first, err := time.ParseInLocation("2006-01-02 15:04", r.List[0].Time, before.Location())
	if err != nil {
		return nil, fmt.Errorf("上市月份日期无效: %w", err)
	}
	start := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, before.Location())
	calendar.Range(start, before, func(day time.Time) bool {
		var trades *TradeResp
		for retry := 0; retry < 3; retry++ {
			trades, err = c.GetHistoryTradeDay(day.Format("20060102"), code)
			if err == nil {
				break
			}
		}
		if err != nil {
			return false
		}
		out.List = append(out.List, trades.List...)
		return true
	})
	if err != nil {
		return nil, err
	}
	out.Count = len(out.List)
	return out, nil
}

var _ TradingCalendar = (*Workday)(nil)
