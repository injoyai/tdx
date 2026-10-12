package tdx

import (
	"errors"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func (c *Conn) GetKlineRange(code, startDate, endDate string) (*MarketKlineResp, error) {
	return c.queryClient().GetKlineRange(code, startDate, endDate)
}

// GetKlineRange 查询包含两端日期的一分钟线，扩展使用原固定周期区间协议。
func (c *MarketClient) GetKlineRange(code, startDate, endDate string) (*MarketKlineResp, error) {
	start, err := parseMarketDate(startDate)
	if err != nil {
		return nil, err
	}
	end, err := parseMarketDate(endDate)
	if err != nil {
		return nil, err
	}
	if start > end {
		return nil, errors.New("起始日期不能晚于结束日期")
	}
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	from, _ := time.Parse("20060102", startDate)
	to, _ := time.Parse("20060102", endDate)
	fromDay, toDay := from.Format("2006-01-02"), to.Format("2006-01-02")
	var out *MarketKlineResp
	if info.Protocol == protocol.ProtocolStandard {
		out, err = c.GetKlineUntil(protocol.TypeKlineMinute, code, func(k MarketKline) bool { return strings.SplitN(k.Time, " ", 2)[0] < fromDay })
		if err != nil {
			return nil, err
		}
	} else {
		link, err := c.connection(i.Exchange)
		if err != nil {
			return nil, err
		}
		r, err := link.getExtendedKlineRange(i.Exchange.Uint8(), i.Code, start, end)
		if err != nil {
			return nil, err
		}
		out = &MarketKlineResp{Instrument: i, Category: protocol.TypeKlineMinute, VolumeUnit: VolumeUnknown}
		for j := range r {
			k := &r[j]
			out.List = append(out.List, MarketKline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: int64(k.Trade), RangeExtended: k})
		}
	}
	items := make([]MarketKline, 0, len(out.List))
	for _, k := range out.List {
		day := strings.SplitN(k.Time, " ", 2)[0]
		if day >= fromDay && day <= toDay {
			items = append(items, k)
		}
	}
	out.List, out.Count = items, len(items)
	return out, nil
}
