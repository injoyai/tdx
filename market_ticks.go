package tdx

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parseMarketDate(date string) (uint32, error) {
	if _, err := time.Parse("20060102", date); err != nil {
		return 0, fmt.Errorf("日期必须为有效 YYYYMMDD: %w", err)
	}
	n, err := strconv.ParseUint(date, 10, 32)
	return uint32(n), err
}

func (c *MarketClient) GetMinute(code string) (*MinuteResp, error) {
	return c.getMinute("", code)
}

func (c *MarketClient) GetHistoryMinute(date, code string) (*MinuteResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.getMinute(date, code)
}

func (c *MarketClient) getMinute(date, code string) (*MinuteResp, error) {
	i, _, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	link, err := c.connection(i.Exchange)
	if err != nil {
		return nil, err
	}
	return link.minutePage(date, code)
}

func (c *MarketClient) GetTrade(code string, start, count uint16) (*TradeResp, error) {
	return c.getTrade("", code, start, count, false)
}

func (c *MarketClient) GetHistoryTrade(date, code string, start, count uint16) (*TradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.getTrade(date, code, start, count, true)
}

func (c *MarketClient) getTrade(date, code string, start, count uint16, checkDay bool) (*TradeResp, error) {
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
	link, err := c.connection(i.Exchange)
	if err != nil {
		return nil, err
	}
	return link.tradePage(date, code, start, count)
}
