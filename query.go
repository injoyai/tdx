package tdx

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func (q *queries) GetKlineMinute(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKlineMinute, code, start, count)
}

func (q *queries) GetKlineMinuteAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKlineMinute, code)
}

func (q *queries) GetKlineMinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKlineMinute, code, stop)
}

func (q *queries) GetKline5Minute(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKline5Minute, code, start, count)
}

func (q *queries) GetKline5MinuteAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKline5Minute, code)
}

func (q *queries) GetKline5MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKline5Minute, code, stop)
}

func (q *queries) GetKline15Minute(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKline15Minute, code, start, count)
}

func (q *queries) GetKline15MinuteAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKline15Minute, code)
}

func (q *queries) GetKline15MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKline15Minute, code, stop)
}

func (q *queries) GetKline30Minute(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKline30Minute, code, start, count)
}

func (q *queries) GetKline30MinuteAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKline30Minute, code)
}

func (q *queries) GetKline30MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKline30Minute, code, stop)
}

func (q *queries) GetKline60Minute(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKline60Minute, code, start, count)
}

func (q *queries) GetKline60MinuteAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKline60Minute, code)
}

func (q *queries) GetKline60MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKline60Minute, code, stop)
}

func (q *queries) GetKlineDay(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKlineDay, code, start, count)
}

func (q *queries) GetKlineDayAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKlineDay, code)
}

func (q *queries) GetKlineDayUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKlineDay, code, stop)
}

func (q *queries) GetKlineWeek(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKlineWeek, code, start, count)
}

func (q *queries) GetKlineWeekAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKlineWeek, code)
}

func (q *queries) GetKlineWeekUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKlineWeek, code, stop)
}

func (q *queries) GetKlineMonth(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKlineMonth, code, start, count)
}

func (q *queries) GetKlineMonthAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKlineMonth, code)
}

func (q *queries) GetKlineMonthUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKlineMonth, code, stop)
}

func (q *queries) GetKlineQuarter(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKlineQuarter, code, start, count)
}

func (q *queries) GetKlineQuarterAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKlineQuarter, code)
}

func (q *queries) GetKlineQuarterUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKlineQuarter, code, stop)
}

func (q *queries) GetKlineYear(code string, start, count uint16) (*KlineResp, error) {
	return q.source.GetKline(protocol.TypeKlineYear, code, start, count)
}

func (q *queries) GetKlineYearAll(code string) (*KlineResp, error) {
	return q.GetKlineAll(protocol.TypeKlineYear, code)
}

func (q *queries) GetKlineYearUntil(code string, stop func(Kline) bool) (*KlineResp, error) {
	return q.GetKlineUntil(protocol.TypeKlineYear, code, stop)
}

// GetKlineUntil 由近到远分页，返回时间升序，包含首次命中停止条件的记录。
func (q *queries) GetKlineUntil(category uint8, code string, stop func(Kline) bool) (*KlineResp, error) {
	if stop == nil {
		return nil, errors.New("K线停止条件不能为空")
	}
	var out *KlineResp
	var last *protocol.Kline
	for start := 0; start <= 65535; start += 800 {
		r, err := q.source.GetKline(category, code, uint16(start), 800)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = &KlineResp{Instrument: r.Instrument, Category: r.Category, TimeZone: r.TimeZone}
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

func (q *queries) GetKlineAll(category uint8, code string) (*KlineResp, error) {
	return q.GetKlineUntil(category, code, func(Kline) bool { return false })
}

func (q *queries) GetMinute(code string) (*MinuteResp, error) {
	return q.getMinute("", code)
}

func (q *queries) GetHistoryMinute(date, code string) (*MinuteResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return q.getMinute(date, code)
}

func (q *queries) getMinute(date, code string) (*MinuteResp, error) {
	return q.source.minutePage(date, code)
}

func (q *queries) GetTrade(code string, start, count uint16) (*TradeResp, error) {
	return q.getTrade("", code, start, count, false)
}

func (q *queries) GetHistoryTrade(date, code string, start, count uint16) (*TradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return q.getTrade(date, code, start, count, true)
}

func (q *queries) getTrade(date, code string, start, count uint16, checkDay bool) (*TradeResp, error) {
	i, info, err := instrumentInfo(code)
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
	out := &TradeResp{Instrument: i, Date: date}
	if info.Protocol == protocol.ProtocolStandard {
		out.TimeZone = protocol.Location().String()
	}
	if date != "" && checkDay {
		day, _ := time.Parse("20060102", date)
		target := day.Format(time.DateOnly)
		// 无对应日线时，服务器仍可能返回异常分笔，必须先确认交易日期。
		days, err := q.GetKlineDayUntil(code, func(k Kline) bool {
			d, _, _ := strings.Cut(k.Time, " ")
			return d <= target
		})
		if err != nil {
			return nil, fmt.Errorf("%s %s 查询分笔前检查日线: %w", code, date, err)
		}
		if len(days.List) == 0 {
			return out, nil
		}
		sourceDate, _, _ := strings.Cut(days.List[0].Time, " ")
		if sourceDate != target {
			return out, nil
		}
	}
	return q.source.tradePage(date, code, start, count)
}

// TradingCalendar 由调用方提供证券对应市场的交易日；中国市场历史日历实现位于 extend/workday。
type TradingCalendar interface {
	Range(start, end time.Time, visit func(time.Time) bool)
}

func (q *queries) GetTradeAll(code string) (*TradeResp, error) {
	return q.tradeAll("", code, false)
}
func (q *queries) GetHistoryTradeDay(date, code string) (*TradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return q.tradeAll(date, code, true)
}

func (q *queries) tradeAll(date, code string, checkDay bool) (*TradeResp, error) {
	size := 1800
	if date != "" {
		size = 2000
	}
	var out *TradeResp
	for start := 0; start <= 65535; start += size {
		r, err := q.getTrade(date, code, uint16(start), uint16(size), checkDay && start == 0)
		if err != nil {
			return nil, err
		}
		if r.Count > size {
			return nil, errors.New("分笔响应数量超过请求数量")
		}
		if out == nil {
			out = &TradeResp{Instrument: r.Instrument, Date: r.Date, TimeZone: r.TimeZone}
		}
		out.List = append(r.List, out.List...)
		out.Count = len(out.List)
		if r.Count < size {
			return out, nil
		}
	}
	return nil, errors.New("分笔分页偏移超过协议 uint16 上限，历史未完整读取")
}

func (q *queries) GetHistoryTradeFull(code string, calendar TradingCalendar) (*TradeResp, error) {
	return q.GetHistoryTradeBefore(code, calendar, time.Now())
}

// GetHistoryTradeBefore 按给定日历获取最早可用月线所在月份起的整日分笔；before 沿用日历的结束边界。
func (q *queries) GetHistoryTradeBefore(code string, calendar TradingCalendar, before time.Time) (*TradeResp, error) {
	if calendar == nil {
		return nil, errors.New("交易日历不能为空")
	}
	if v := reflect.ValueOf(calendar); v.Kind() == reflect.Pointer && v.IsNil() {
		return nil, errors.New("交易日历不能为空")
	}
	if before.IsZero() {
		return nil, errors.New("结束时间不能为空")
	}
	r, err := q.GetKlineMonthAll(code)
	if err != nil {
		return nil, err
	}
	out := &TradeResp{Instrument: r.Instrument, TimeZone: r.TimeZone, List: []Trade{}}
	if r.Count == 0 {
		return out, nil
	}
	firstDate, _, _ := strings.Cut(r.List[0].Time, " ")
	first, err := time.ParseInLocation(time.DateOnly, firstDate, before.Location())
	if err != nil {
		return nil, fmt.Errorf("上市月份日期无效: %w", err)
	}
	start := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, before.Location())
	// 一轮历史查询共用日线日期校验，避免逐日从最新页重新扫描。
	days, err := q.GetKlineDayUntil(code, func(k Kline) bool {
		date, _, _ := strings.Cut(k.Time, " ")
		return date <= start.Format(time.DateOnly)
	})
	if err != nil {
		return nil, fmt.Errorf("%s 查询分笔前检查日线: %w", code, err)
	}
	dates := make(map[string]bool, len(days.List))
	for _, k := range days.List {
		date, _, _ := strings.Cut(k.Time, " ")
		dates[strings.ReplaceAll(date, "-", "")] = true
	}
	calendar.Range(start, before, func(day time.Time) bool {
		date := day.Format("20060102")
		if !dates[date] {
			return true
		}
		var trades *TradeResp
		for retry := 0; retry < 3; retry++ {
			trades, err = q.tradeAll(date, code, false)
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

func (q *queries) GetKlineMinute241Until(code string, stop func(Kline) bool) (*KlineResp, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	if info.Protocol != protocol.ProtocolStandard || (!protocol.IsStock(standardCode(i)) && !protocol.IsETF(standardCode(i))) {
		return nil, fmt.Errorf("241分钟线仅适用于标准股票/ETF；其他品种使用 GetKlineMinuteUntil")
	}
	resp, err := q.GetKlineMinuteUntil(code, stop)
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
			if v.Time.Format(time.DateOnly) == time.Now().In(protocol.Location()).Format(time.DateOnly) {
				tr, err = q.GetTradeAll(code)
			} else {
				tr, err = q.GetHistoryTradeDay(v.Time.Format("20060102"), code)
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
	for j := range ks {
		ks[j].Volume, err = scaleVolume(ks[j].Volume, protocol.SharesPerLot)
		if err != nil {
			return nil, err
		}
	}
	resp.List = ks
	resp.Count = len(ks)
	return resp, nil
}
