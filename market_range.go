package tdx

import (
	"errors"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func (c *Conn) GetKlineRange(code, startDate, endDate string) (*KlineResp, error) {
	return c.queryClient().GetKlineRange(code, startDate, endDate)
}

// GetKlineRange 查询包含两端日期的一分钟线，由对应协议客户端读取。
func (c *MarketClient) GetKlineRange(code, startDate, endDate string) (*KlineResp, error) {
	if _, _, _, _, err := klineRangeDates(startDate, endDate); err != nil {
		return nil, err
	}
	i, _, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	link, err := c.connection(i.Exchange)
	if err != nil {
		return nil, err
	}
	return link.GetKlineRange(code, startDate, endDate)
}

func klineRangeDates(startDate, endDate string) (uint32, uint32, string, string, error) {
	start, err := parseMarketDate(startDate)
	if err != nil {
		return 0, 0, "", "", err
	}
	end, err := parseMarketDate(endDate)
	if err != nil {
		return 0, 0, "", "", err
	}
	if start > end {
		return 0, 0, "", "", errors.New("起始日期不能晚于结束日期")
	}
	from, _ := time.Parse("20060102", startDate)
	to, _ := time.Parse("20060102", endDate)
	return start, end, from.Format(time.DateOnly), to.Format(time.DateOnly), nil
}

// GetKlineRange 通过标准一分钟分页筛选日期区间，包含两端日期。
func (c *StandardClient) GetKlineRange(code, startDate, endDate string) (*KlineResp, error) {
	_, _, fromDay, toDay, err := klineRangeDates(startDate, endDate)
	if err != nil {
		return nil, err
	}
	if _, _, err := c.protocolInstrument(code, false); err != nil {
		return nil, err
	}
	out, err := c.GetKlineUntil(protocol.TypeKlineMinute, code, func(k Kline) bool { return strings.SplitN(k.Time, " ", 2)[0] < fromDay })
	if err != nil {
		return nil, err
	}
	return filterKlineRange(out, fromDay, toDay), nil
}

// GetKlineRange 使用扩展固定周期区间协议，原持仓和结算价保留在 RangeExtended。
func (c *ExtendedClient) GetKlineRange(code, startDate, endDate string) (*KlineResp, error) {
	start, end, fromDay, toDay, err := klineRangeDates(startDate, endDate)
	if err != nil {
		return nil, err
	}
	i, _, err := c.protocolInstrument(code, true)
	if err != nil {
		return nil, err
	}
	r, err := c.getExtendedKlineRange(i.Exchange.Uint8(), i.Code, start, end)
	if err != nil {
		return nil, err
	}
	out := &KlineResp{Instrument: i, Category: protocol.TypeKlineMinute, VolumeUnit: VolumeUnknown}
	for j := range r {
		k := &r[j]
		out.List = append(out.List, Kline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: int64(k.Trade), RangeExtended: k})
	}
	return filterKlineRange(out, fromDay, toDay), nil
}

func filterKlineRange(out *KlineResp, fromDay, toDay string) *KlineResp {
	items := make([]Kline, 0, len(out.List))
	for _, k := range out.List {
		day := strings.SplitN(k.Time, " ", 2)[0]
		if day >= fromDay && day <= toDay {
			items = append(items, k)
		}
	}
	out.List, out.Count = items, len(items)
	return out
}
