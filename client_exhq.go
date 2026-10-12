package tdx

import (
	"errors"
	"strings"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/module/tcp"
	"github.com/injoyai/tdx/v2/protocol"
)

// DialExHqDefault 连接默认扩展行情服务器(轮询 ExHosts)。
func DialExHqDefault(op ...client.Option) (*Client, error) {
	op = append([]client.Option{WithRedial()}, op...)
	return DialExHqHosts(ExHosts, op...)
}

// DialExHq 连接指定扩展行情服务器。
func DialExHq(addr string, op ...client.Option) (*Client, error) {
	if !strings.Contains(addr, ":") {
		addr += ":" + ExPort
	}
	return dialExHqWith(tcp.NewDial(addr), op...)
}

// DialExHqHosts 连接扩展行情服务器,多服务器轮询。
func DialExHqHosts(hosts []string, op ...client.Option) (*Client, error) {
	return dialExHqWith(NewExRangeDial(hosts), op...)
}

// dialExHqWith 建立扩展行情连接:握手用 ExSetup,心跳用品种数量请求。
func dialExHqWith(dial ios.DialFunc, op ...client.Option) (cli *Client, err error) {
	return dialWith(dial, true, op...)
}

// ---- 扩展行情 API ----

// getExtendedMarkets 市场代码表。
func (this *Client) getExtendedMarkets() ([]protocol.ExMarket, error) {
	r, err := this.SendFrame(protocol.MEx.FrameMarkets())
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMarket), nil
}

// getExtendedCount 全部品种数量。
func (this *Client) getExtendedCount() (int, error) {
	r, err := this.SendFrame(protocol.MEx.FrameCount())
	if err != nil {
		return 0, err
	}
	return r.(int), nil
}

// getExtendedInstruments 按全局序号分页的品种列表。
func (this *Client) getExtendedInstruments(start uint32, count uint16) ([]protocol.ExInstrument, error) {
	r, err := this.SendFrame(protocol.MEx.FrameInstrument(start, count))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExInstrument), nil
}

// getExtendedQuote 单品种五档行情。
func (this *Client) getExtendedQuote(market uint8, code string) (*protocol.ExQuote, error) {
	r, err := this.SendFrame(protocol.MEx.FrameQuote(market, code))
	if err != nil {
		return nil, err
	}
	return r.(*protocol.ExQuote), nil
}

// GetQuoteList 批量行情列表(category 2=港股 3=期货)。
func (this *Client) GetQuoteList(market, category uint8, start, count uint16) ([]protocol.ExQuoteListItem, error) {
	if !this.extended {
		return nil, errors.New("市场行情列表需要扩展连接")
	}
	r, err := this.SendFrame(protocol.MEx.FrameQuoteList(market, start, count), protocol.ExQuoteListCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExQuoteListItem), nil
}

// getExtendedKline K线(category 同标准 KLINE 类型)。
func (this *Client) getExtendedKline(category, market uint8, code string, start, count uint16) ([]protocol.ExKline, error) {
	r, err := this.SendFrame(protocol.MEx.FrameBars(category, market, code, start, count), protocol.ExBarsCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExKline), nil
}

// getExtendedMinute 当日分时。
func (this *Client) getExtendedMinute(market uint8, code string) ([]protocol.ExMinuteTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameMinute(market, code))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// getExtendedHistoryMinute 历史分时(date=YYYYMMDD)。
func (this *Client) getExtendedHistoryMinute(market uint8, code string, date uint32) ([]protocol.ExMinuteTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameHistMinute(market, code, date))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// getExtendedTrade 当日分笔成交。
func (this *Client) getExtendedTrade(market uint8, code string, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameTrade(market, code, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// getExtendedHistoryTrade 历史分笔成交(date=YYYYMMDD)。
func (this *Client) getExtendedHistoryTrade(market uint8, code string, date uint32, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameHistTrade(market, code, date, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// getExtendedKlineRange 历史K线区间(date/date2=YYYYMMDD)。
func (this *Client) getExtendedKlineRange(market uint8, code string, date, date2 uint32) ([]protocol.ExRangeKline, error) {
	r, err := this.SendFrame(protocol.MEx.FrameBarsRange(market, code, date, date2))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExRangeKline), nil
}
