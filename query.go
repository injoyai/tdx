package tdx

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func (c *queries) GetKlineMinute(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKlineMinute, code, start, count)
}

func (c *queries) GetKlineMinuteAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineMinute, code)
}

func (c *queries) GetKlineMinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineMinute, code, stop)
}

func (c *queries) GetKline5Minute(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKline5Minute, code, start, count)
}

func (c *queries) GetKline5MinuteAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline5Minute, code)
}

func (c *queries) GetKline5MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline5Minute, code, stop)
}

func (c *queries) GetKline15Minute(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKline15Minute, code, start, count)
}

func (c *queries) GetKline15MinuteAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline15Minute, code)
}

func (c *queries) GetKline15MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline15Minute, code, stop)
}

func (c *queries) GetKline30Minute(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKline30Minute, code, start, count)
}

func (c *queries) GetKline30MinuteAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline30Minute, code)
}

func (c *queries) GetKline30MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline30Minute, code, stop)
}

func (c *queries) GetKline60Minute(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKline60Minute, code, start, count)
}

func (c *queries) GetKline60MinuteAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline60Minute, code)
}

func (c *queries) GetKline60MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline60Minute, code, stop)
}

func (c *queries) GetKlineDay(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKlineDay, code, start, count)
}

func (c *queries) GetKlineDayAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineDay, code)
}

func (c *queries) GetKlineDayUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineDay, code, stop)
}

func (c *queries) GetKlineWeek(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKlineWeek, code, start, count)
}

func (c *queries) GetKlineWeekAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineWeek, code)
}

func (c *queries) GetKlineWeekUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineWeek, code, stop)
}

func (c *queries) GetKlineMonth(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKlineMonth, code, start, count)
}

func (c *queries) GetKlineMonthAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineMonth, code)
}

func (c *queries) GetKlineMonthUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineMonth, code, stop)
}

func (c *queries) GetKlineQuarter(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKlineQuarter, code, start, count)
}

func (c *queries) GetKlineQuarterAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineQuarter, code)
}

func (c *queries) GetKlineQuarterUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineQuarter, code, stop)
}

func (c *queries) GetKlineYear(code string, start, count uint16) (*KlineResp, error) {
	return c.source.GetKline(protocol.TypeKlineYear, code, start, count)
}

func (c *queries) GetKlineYearAll(code string) (*KlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineYear, code)
}

func (c *queries) GetKlineYearUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineYear, code, stop)
}

// GetKlineUntil 由近到远分页，返回时间升序，包含首次命中停止条件的记录。
func (c *queries) GetKlineUntil(category uint8, code string, stop func(Kline) bool) (*KlineResp, error) {
	if stop == nil {
		return nil, errors.New("K线停止条件不能为空")
	}
	var out *KlineResp
	var last *protocol.Kline
	for start := 0; start <= 65535; start += 800 {
		r, err := c.source.GetKline(category, code, uint16(start), 800)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = &KlineResp{Instrument: r.Instrument, Category: r.Category, VolumeUnit: r.VolumeUnit}
		}
		if len(r.List) > 0 && r.List[0].Standard != nil {
			if last != nil {
				last.Last = r.List[len(r.List)-1].Standard.Close
			}
			last = r.List[0].Standard
		}
		matched := false
		for j := len(r.List) - 1; j >= 0; j-- {
			if stop(r.List[j]) {
				r.List = r.List[j:]
				matched = true
				break
			}
		}
		out.List = append(r.List, out.List...)
		out.Count = len(out.List)
		if matched || r.Count < 800 {
			return out, nil
		}
	}
	return nil, errors.New("K线分页偏移超过协议 uint16 上限，历史未完整读取")
}

func (c *queries) GetKlineAll(category uint8, code string) (*KlineResp, error) {
	return c.GetKlineUntil(category, code, func(Kline) bool { return false })
}

func (c *queries) GetMinute(code string) (*MinuteResp, error) {
	return c.getMinute("", code)
}

func (c *queries) GetHistoryMinute(date, code string) (*MinuteResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.getMinute(date, code)
}

func (c *queries) getMinute(date, code string) (*MinuteResp, error) {
	return c.source.minutePage(date, code)
}

func (c *queries) GetTrade(code string, start, count uint16) (*TradeResp, error) {
	return c.getTrade("", code, start, count, false)
}

func (c *queries) GetHistoryTrade(date, code string, start, count uint16) (*TradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.getTrade(date, code, start, count, true)
}

func (c *queries) getTrade(date, code string, start, count uint16, checkDay bool) (*TradeResp, error) {
	i, _, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	limit := uint16(1800)
	if date != "" {
		limit = 2000
	}
	if count == 0 || count > limit {
		return nil, fmt.Errorf("分笔单次数量必须为1..%d", limit)
	}
	out := &TradeResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	if date != "" && checkDay {
		day, _ := time.Parse("20060102", date)
		target := day.Format(time.DateOnly)
		// 无对应日线时，服务器仍可能返回异常分笔，必须先确认交易日期。
		days, err := c.GetKlineDayUntil(code, func(k Kline) bool {
			d, _, _ := strings.Cut(k.Time, " ")
			return d <= target
		})
		if err != nil {
			return nil, fmt.Errorf("%s %s 查询分笔前检查日线: %w", code, date, err)
		}
		if len(days.List) == 0 || !strings.HasPrefix(days.List[0].Time, target+" ") {
			return out, nil
		}
	}
	return c.source.tradePage(date, code, start, count)
}

// TradingCalendar 由调用方提供证券对应市场的交易日；Workday 是沪深日历。
type TradingCalendar interface {
	Range(start, end time.Time, visit func(time.Time) bool)
}

func (c *queries) GetTradeAll(code string) (*TradeResp, error) {
	return c.tradeAll("", code)
}
func (c *queries) GetHistoryTradeDay(date, code string) (*TradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.tradeAll(date, code)
}

func (c *queries) tradeAll(date, code string) (*TradeResp, error) {
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

func (c *queries) GetHistoryTradeFull(code string, calendar TradingCalendar) (*TradeResp, error) {
	return c.GetHistoryTradeBefore(code, calendar, time.Now())
}

// GetHistoryTradeBefore 按给定日历获取最早可用月线所在月份起的整日分笔；before 沿用日历的结束边界。
func (c *queries) GetHistoryTradeBefore(code string, calendar TradingCalendar, before time.Time) (*TradeResp, error) {
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

func (c *queries) GetKlineMinute241Until(code string, stop func(Kline) bool) (*KlineResp, error) {
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
