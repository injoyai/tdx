package tdx

import "github.com/injoyai/tdx/v2/protocol"

func (c *MarketClient) GetKlineMinute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKlineMinute, code, start, count)
}

func (c *Conn) GetKlineMinute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMinute(code, start, count)
}

func (c *MarketClient) GetKlineMinuteAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineMinute, code)
}

func (c *Conn) GetKlineMinuteAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMinuteAll(code)
}

func (c *MarketClient) GetKlineMinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineMinute, code, stop)
}

func (c *Conn) GetKlineMinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMinuteUntil(code, stop)
}

func (c *MarketClient) GetKline5Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKline5Minute, code, start, count)
}

func (c *Conn) GetKline5Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKline5Minute(code, start, count)
}

func (c *MarketClient) GetKline5MinuteAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline5Minute, code)
}

func (c *Conn) GetKline5MinuteAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKline5MinuteAll(code)
}

func (c *MarketClient) GetKline5MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline5Minute, code, stop)
}

func (c *Conn) GetKline5MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKline5MinuteUntil(code, stop)
}

func (c *MarketClient) GetKline15Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKline15Minute, code, start, count)
}

func (c *Conn) GetKline15Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKline15Minute(code, start, count)
}

func (c *MarketClient) GetKline15MinuteAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline15Minute, code)
}

func (c *Conn) GetKline15MinuteAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKline15MinuteAll(code)
}

func (c *MarketClient) GetKline15MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline15Minute, code, stop)
}

func (c *Conn) GetKline15MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKline15MinuteUntil(code, stop)
}

func (c *MarketClient) GetKline30Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKline30Minute, code, start, count)
}

func (c *Conn) GetKline30Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKline30Minute(code, start, count)
}

func (c *MarketClient) GetKline30MinuteAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline30Minute, code)
}

func (c *Conn) GetKline30MinuteAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKline30MinuteAll(code)
}

func (c *MarketClient) GetKline30MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline30Minute, code, stop)
}

func (c *Conn) GetKline30MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKline30MinuteUntil(code, stop)
}

func (c *MarketClient) GetKline60Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKline60Minute, code, start, count)
}

func (c *Conn) GetKline60Minute(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKline60Minute(code, start, count)
}

func (c *MarketClient) GetKline60MinuteAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKline60Minute, code)
}

func (c *Conn) GetKline60MinuteAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKline60MinuteAll(code)
}

func (c *MarketClient) GetKline60MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKline60Minute, code, stop)
}

func (c *Conn) GetKline60MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKline60MinuteUntil(code, stop)
}

func (c *MarketClient) GetKlineDay(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKlineDay, code, start, count)
}

func (c *Conn) GetKlineDay(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineDay(code, start, count)
}

func (c *MarketClient) GetKlineDayAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineDay, code)
}

func (c *Conn) GetKlineDayAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineDayAll(code)
}

func (c *MarketClient) GetKlineDayUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineDay, code, stop)
}

func (c *Conn) GetKlineDayUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineDayUntil(code, stop)
}

func (c *MarketClient) GetKlineWeek(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKlineWeek, code, start, count)
}

func (c *Conn) GetKlineWeek(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineWeek(code, start, count)
}

func (c *MarketClient) GetKlineWeekAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineWeek, code)
}

func (c *Conn) GetKlineWeekAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineWeekAll(code)
}

func (c *MarketClient) GetKlineWeekUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineWeek, code, stop)
}

func (c *Conn) GetKlineWeekUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineWeekUntil(code, stop)
}

func (c *MarketClient) GetKlineMonth(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKlineMonth, code, start, count)
}

func (c *Conn) GetKlineMonth(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMonth(code, start, count)
}

func (c *MarketClient) GetKlineMonthAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineMonth, code)
}

func (c *Conn) GetKlineMonthAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMonthAll(code)
}

func (c *MarketClient) GetKlineMonthUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineMonth, code, stop)
}

func (c *Conn) GetKlineMonthUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMonthUntil(code, stop)
}

func (c *MarketClient) GetKlineQuarter(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKlineQuarter, code, start, count)
}

func (c *Conn) GetKlineQuarter(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineQuarter(code, start, count)
}

func (c *MarketClient) GetKlineQuarterAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineQuarter, code)
}

func (c *Conn) GetKlineQuarterAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineQuarterAll(code)
}

func (c *MarketClient) GetKlineQuarterUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineQuarter, code, stop)
}

func (c *Conn) GetKlineQuarterUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineQuarterUntil(code, stop)
}

func (c *MarketClient) GetKlineYear(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.GetKline(protocol.TypeKlineYear, code, start, count)
}

func (c *Conn) GetKlineYear(code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineYear(code, start, count)
}

func (c *MarketClient) GetKlineYearAll(code string) (*MarketKlineResp, error) {
	return c.GetKlineAll(protocol.TypeKlineYear, code)
}

func (c *Conn) GetKlineYearAll(code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineYearAll(code)
}

func (c *MarketClient) GetKlineYearUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineYear, code, stop)
}

func (c *Conn) GetKlineYearUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineYearUntil(code, stop)
}

func (c *Conn) GetKlineUntil(category uint8, code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineUntil(category, code, stop)
}

func (c *Conn) GetKlineAll(category uint8, code string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineAll(category, code)
}

func (c *Conn) GetKlineMinute241Until(code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineMinute241Until(code, stop)
}
