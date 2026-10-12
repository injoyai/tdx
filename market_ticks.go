package tdx

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func parseMarketDate(date string) (uint32, error) {
	if _, err := time.Parse("20060102", date); err != nil {
		return 0, fmt.Errorf("日期必须为有效 YYYYMMDD: %w", err)
	}
	n, err := strconv.ParseUint(date, 10, 32)
	return uint32(n), err
}

func (c *MarketClient) GetMinute(code string) (*MarketMinuteResp, error) {
	return c.getMinute("", code)
}

func (c *MarketClient) GetHistoryMinute(date, code string) (*MarketMinuteResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.getMinute(date, code)
}

func (c *MarketClient) getMinute(date, code string) (*MarketMinuteResp, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	link, err := c.connection(i.Exchange)
	if err != nil {
		return nil, err
	}
	out := &MarketMinuteResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	if info.Protocol == protocol.ProtocolStandard {
		var r *protocol.MinuteResp
		if date == "" {
			r, err = link.getStandardMinute(standardCode(i))
		} else {
			r, err = link.getStandardHistoryMinute(date, standardCode(i))
		}
		if err != nil {
			return nil, err
		}
		for j := range r.List {
			k := &r.List[j]
			out.List = append(out.List, MarketMinute{Time: k.Time, Price: k.Price.Float64(), Volume: int64(k.Number), Standard: k})
		}
	} else {
		var ticks []protocol.ExMinuteTick
		if date == "" {
			ticks, err = link.getExtendedMinute(i.Exchange.Uint8(), i.Code)
		} else {
			n, _ := parseMarketDate(date)
			ticks, err = link.getExtendedHistoryMinute(i.Exchange.Uint8(), i.Code, n)
		}
		if err != nil {
			return nil, err
		}
		for j := range ticks {
			k := &ticks[j]
			out.List = append(out.List, MarketMinute{Time: fmt.Sprintf("%02d:%02d", k.Hour, k.Minute), Price: k.Price, Volume: int64(k.Volume), Extended: k})
		}
	}
	out.Count = len(out.List)
	return out, nil
}

func (c *MarketClient) GetTrade(code string, start, count uint16) (*MarketTradeResp, error) {
	return c.getTrade("", code, start, count, false)
}

func (c *MarketClient) GetHistoryTrade(date, code string, start, count uint16) (*MarketTradeResp, error) {
	if _, err := parseMarketDate(date); err != nil {
		return nil, err
	}
	return c.getTrade(date, code, start, count, true)
}

func (c *MarketClient) getTrade(date, code string, start, count uint16, checkDay bool) (*MarketTradeResp, error) {
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
	out := &MarketTradeResp{Instrument: i, Date: date, VolumeUnit: VolumeUnknown}
	if date != "" && checkDay {
		day, _ := time.Parse("20060102", date)
		target := day.Format(time.DateOnly)
		// 无对应日线时，服务器仍可能返回异常分笔，必须先确认交易日期。
		days, err := c.GetKlineDayUntil(code, func(k MarketKline) bool {
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
	if info.Protocol == protocol.ProtocolStandard {
		var r *protocol.TradeResp
		if date == "" {
			r, err = link.getStandardTrade(standardCode(i), start, count)
		} else {
			r, err = link.getStandardHistoryTrade(date, standardCode(i), start, count)
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
			out.List = append(out.List, MarketTrade{Date: date, Time: k.Time.Format("15:04:05"), Price: &price, Volume: int64(k.Volume), Direction: direction, Standard: k})
		}
	} else {
		var ticks []protocol.ExTradeTick
		if date == "" {
			ticks, err = link.getExtendedTrade(i.Exchange.Uint8(), i.Code, start, count)
		} else {
			n, _ := parseMarketDate(date)
			ticks, err = link.getExtendedHistoryTrade(i.Exchange.Uint8(), i.Code, n, start, count)
		}
		if err != nil {
			return nil, err
		}
		// shortcut: 扩展分笔价格尺度未确认，取得各市场交叉验证样本后填充公共 Price。
		for j := range ticks {
			k := &ticks[j]
			out.List = append(out.List, MarketTrade{Date: date, Time: fmt.Sprintf("%02d:%02d:%02d", k.Hour, k.Minute, k.Second), Volume: int64(k.Volume), Direction: k.Direction, Extended: k})
		}
	}
	out.Count = len(out.List)
	return out, nil
}
