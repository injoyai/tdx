package tdx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// ExConn 是绑定扩展行情协议的业务连接，实现 Client；财务、F10 等标准专用查询返回不支持错误。
// DialExHq* 直接返回该类型，不会建立标准连接。
type ExConn struct {
	*conn
	*queries
}

// GetKline 请求扩展 K 线，保留持仓、结算价等扩展字段。
func (c *ExConn) GetKline(category uint8, code string, start, count uint16) (*KlineResp, error) {
	out, _, err := klineRequest(code, category, count)
	if err != nil {
		return nil, err
	}
	i, _, err := protocolInstrument(code, true)
	if err != nil {
		return nil, err
	}
	bars, err := c.getExtendedKline(out.Category, i.Exchange.Uint8(), i.Code, start, count)
	if err != nil {
		return nil, err
	}
	for j := range bars {
		k := &bars[j]
		out.List = append(out.List, Kline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: int64(k.Trade), Extended: k})
	}
	if len(out.List) > int(count) {
		return nil, errors.New("K线响应数量超过请求数量")
	}
	out.Count = len(out.List)
	return out, nil
}

// GetQuote 逐品种获取扩展五档报价，任一请求失败不返回部分结果。
func (c *ExConn) GetQuote(codes ...string) ([]Quote, error) {
	instruments := make([]protocol.Instrument, len(codes))
	for j, code := range codes {
		i, _, err := protocolInstrument(code, true)
		if err != nil {
			return nil, err
		}
		instruments[j] = i
	}
	out := make([]Quote, len(codes))
	for j, i := range instruments {
		q, err := c.getExtendedQuote(i.Exchange.Uint8(), i.Code)
		if err != nil {
			return nil, err
		}
		if q.Market != i.Exchange.Uint8() || q.Code != i.Code {
			return nil, fmt.Errorf("扩展行情响应品种与请求不一致: %s", i)
		}
		u := Quote{Instrument: i, PreClose: q.PreClose, Open: q.Open, High: q.High, Low: q.Low, Price: q.Price, Volume: int64(q.ZongLiang), VolumeUnit: VolumeUnknown, Bid: q.Bid, Ask: q.Ask, Extended: q}
		for n := 0; n < 5; n++ {
			u.BidVolume[n], u.AskVolume[n] = int64(q.BidVol[n]), int64(q.AskVol[n])
		}
		out[j] = u
	}
	return out, nil
}

// GetCode 按扩展市场内偏移分页，不将市场编号当作全局品种偏移。
func (c *ExConn) GetCode(ex protocol.Exchange, start uint32, count uint16) (*CodeResp, error) {
	if err := codePageBounds(ex, start, count); err != nil {
		return nil, err
	}
	// shortcut: 扩展分页逐次扫描全局列表，频繁查询时改为显式快照分页。
	out, err := c.GetCodeAll(ex)
	if err != nil {
		return nil, err
	}
	return codePage(out, start, count), nil
}

// GetCodeAll 扫描扩展全局品种表，再按指定市场过滤；不增加跨查询缓存。
func (c *ExConn) GetCodeAll(ex protocol.Exchange) (*CodeResp, error) {
	if err := protocolExchange(ex, true); err != nil {
		return nil, err
	}
	total, err := c.getExtendedCount()
	if err != nil {
		return nil, err
	}
	out := &CodeResp{List: []Code{}}
	for start := 0; start < total; {
		count := min(800, total-start)
		items, err := c.getExtendedInstruments(uint32(start), uint16(count))
		if err != nil {
			return nil, err
		}
		if len(items) == 0 || len(items) > count {
			return nil, fmt.Errorf("扩展品种分页 %d 不完整: %w", start, io.ErrUnexpectedEOF)
		}
		for j := range items {
			k := &items[j]
			if k.Market == ex.Uint8() {
				out.List = append(out.List, Code{Instrument: protocol.Instrument{Exchange: ex, Code: k.Code}, Name: k.Name, Extended: k})
			}
		}
		start += len(items)
	}
	out.Count = len(out.List)
	return out, nil
}

// GetCount 返回扩展单市场数量，而非服务器全市场总数。
func (c *ExConn) GetCount(ex protocol.Exchange) (int, error) {
	r, err := c.GetCodeAll(ex)
	if err != nil {
		return 0, err
	}
	return r.Count, nil
}

func (c *ExConn) minutePage(date, code string) (*MinuteResp, error) {
	i, _, err := protocolInstrument(code, true)
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

func (c *ExConn) tradePage(date, code string, start, count uint16) (*TradeResp, error) {
	i, _, err := protocolInstrument(code, true)
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

// GetQuoteList 获取扩展市场分页报价，保留原始字段和单位。
func (c *ExConn) GetQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error) {
	if err := protocolExchange(protocol.Exchange(market), true); err != nil {
		return nil, err
	}
	return c.getQuoteList(market, category, start, count)
}

// GetKlineRange 使用扩展固定周期区间协议，原持仓和结算价保留在 RangeExtended。
func (c *ExConn) GetKlineRange(code, startDate, endDate string) (*KlineResp, error) {
	start, end, fromDay, toDay, err := klineRangeDates(startDate, endDate)
	if err != nil {
		return nil, err
	}
	i, _, err := protocolInstrument(code, true)
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

// GetMarkets 返回扩展服务器发现的市场；发现不代表查询路由已经接入。
func (c *ExConn) GetMarkets() ([]Exchange, error) {
	select {
	case <-c.Done():
		return nil, net.ErrClosed
	default:
	}
	if err := protocolExchange(protocol.ExchangeHK, true); err != nil {
		return nil, err
	}
	r, err := c.getExtendedMarkets()
	if err != nil {
		return nil, err
	}
	out := make([]Exchange, 0, len(r))
	for j := range r {
		m := &r[j]
		out = append(out, Exchange{Market: m.Market, Name: m.Name, ShortName: m.ShortName, Protocol: protocol.ProtocolExtended, Extended: m})
	}
	return out, nil
}

// getExtendedMarkets 市场代码表。
func (this *ExConn) getExtendedMarkets() ([]protocol.ExMarket, error) {
	r, err := this.SendFrame(protocol.MEx.FrameMarkets())
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMarket), nil
}

// getExtendedCount 全部品种数量。
func (this *ExConn) getExtendedCount() (int, error) {
	r, err := this.SendFrame(protocol.MEx.FrameCount())
	if err != nil {
		return 0, err
	}
	return r.(int), nil
}

// getExtendedInstruments 按全局序号分页的品种列表。
func (this *ExConn) getExtendedInstruments(start uint32, count uint16) ([]protocol.ExInstrument, error) {
	r, err := this.SendFrame(protocol.MEx.FrameInstrument(start, count))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExInstrument), nil
}

// getExtendedQuote 单品种五档行情。
func (this *ExConn) getExtendedQuote(market uint8, code string) (*protocol.ExQuote, error) {
	r, err := this.SendFrame(protocol.MEx.FrameQuote(market, code))
	if err != nil {
		return nil, err
	}
	return r.(*protocol.ExQuote), nil
}

// GetQuoteList 批量行情列表(category 2=港股 3=期货)。
func (this *ExConn) getQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error) {
	r, err := this.SendFrame(protocol.MEx.FrameQuoteList(market, start, count), protocol.ExQuoteListCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.QuoteListItem), nil
}

// getExtendedKline K线(category 同标准 KLINE 类型)。
func (this *ExConn) getExtendedKline(category, market uint8, code string, start, count uint16) ([]protocol.ExKline, error) {
	r, err := this.SendFrame(protocol.MEx.FrameBars(category, market, code, start, count), protocol.ExBarsCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExKline), nil
}

// getExtendedMinute 当日分时。
func (this *ExConn) getExtendedMinute(market uint8, code string) ([]protocol.ExMinuteTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameMinute(market, code))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// getExtendedHistoryMinute 历史分时(date=YYYYMMDD)。
func (this *ExConn) getExtendedHistoryMinute(market uint8, code string, date uint32) ([]protocol.ExMinuteTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameHistMinute(market, code, date))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// getExtendedTrade 当日分笔成交。
func (this *ExConn) getExtendedTrade(market uint8, code string, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameTrade(market, code, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// getExtendedHistoryTrade 历史分笔成交(date=YYYYMMDD)。
func (this *ExConn) getExtendedHistoryTrade(market uint8, code string, date uint32, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameHistTrade(market, code, date, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// getExtendedKlineRange 历史K线区间(date/date2=YYYYMMDD)。
func (this *ExConn) getExtendedKlineRange(market uint8, code string, date, date2 uint32) ([]protocol.ExRangeKline, error) {
	r, err := this.SendFrame(protocol.MEx.FrameBarsRange(market, code, date, date2))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExRangeKline), nil
}
func (c *ExConn) decode(f *protocol.Response, val any) (resp any, err error) {
	switch f.Type {
	case protocol.TypeExSetup:
		// 握手响应忽略

	case protocol.TypeExMarkets:
		resp, err = protocol.MEx.DecodeMarkets(f.Data)

	case protocol.TypeExCount:
		resp, err = protocol.MEx.DecodeCount(f.Data)

	case protocol.TypeExInstrument:
		resp, err = protocol.MEx.DecodeInstrument(f.Data)

	case protocol.TypeExQuote:
		resp, err = protocol.MEx.DecodeQuote(f.Data)

	case protocol.TypeExQuoteList:
		resp, err = protocol.MEx.DecodeQuoteList(f.Data, val.(protocol.ExQuoteListCache))

	case protocol.TypeExBars:
		resp, err = protocol.MEx.DecodeBars(f.Data, val.(protocol.ExBarsCache))

	case protocol.TypeExMinute:
		resp, err = protocol.MEx.DecodeMinute(f.Data)

	case protocol.TypeExHistMinute:
		resp, err = protocol.MEx.DecodeHistMinute(f.Data)

	case protocol.TypeExTrade:
		resp, err = protocol.MEx.DecodeTrade(f.Data, val.(protocol.ExTradeCache))

	case protocol.TypeExHistTrade:
		resp, err = protocol.MEx.DecodeHistTrade(f.Data, val.(protocol.ExTradeCache))

	case protocol.TypeExBarsRange:
		resp, err = protocol.MEx.DecodeBarsRange(f.Data)

	default:
		err = fmt.Errorf("通讯类型未解析:0x%X", f.Type)
	}
	return resp, err
}
func newExConn(core *conn) *ExConn {
	c := &ExConn{conn: core}
	c.queries = &queries{source: c}
	return c
}
func (c *ExConn) protocol() protocol.MarketProtocol { return protocol.ProtocolExtended }

func (c *ExConn) GetBlockData(file string) ([]*protocol.Block, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetBlockFileRaw(file string) ([]byte, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error) {
	return "", errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetETFCodeAll() ([]string, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetGbbq(code string) (*protocol.GbbqResp, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetIndexCodeAll() ([]string, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetReportFile(file string) ([]byte, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetSpBlock() ([]*protocol.SpBlock, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetStockCodeAll() ([]string, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetTdxBk() ([]*protocol.TdxBk, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetTdxHy() ([]*protocol.TdxHy, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetTdxStat() ([]*protocol.TdxStat, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetTdxZs() ([]*protocol.TdxZs, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetXgsg() ([]*protocol.TdxXgsg, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (c *ExConn) GetZHBFiles() (map[string][]byte, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

// DialExHqDefault 连接默认扩展行情服务器(轮询 ExHosts)。
func DialExHqDefault(op ...client.Option) (*ExConn, error) {
	op = append([]client.Option{WithRedial()}, op...)
	return DialExHqHosts(ExHosts, op...)
}

// DialExHq 连接指定扩展行情服务器。
func DialExHq(addr string, op ...client.Option) (*ExConn, error) {
	return dialExHqWith(NewTCPDial(withPort(addr, ExPort)), op...)
}

// DialExHqHosts 遍历扩展行情服务器，裸 IP 默认使用端口 7727。
func DialExHqHosts(hosts []string, op ...client.Option) (*ExConn, error) {
	if len(hosts) == 0 {
		hosts = ExHosts
	}
	if len(hosts) == 0 {
		return nil, errors.New("扩展行情服务器列表为空")
	}
	return dialExHqWith(NewRangeDial(hostAddrs(hosts, ExPort)), op...)
}

// dialExHqWith 建立扩展行情连接:握手用 ExSetup,心跳用品种数量请求。
func dialExHqWith(dial ios.DialFunc, op ...client.Option) (cli *ExConn, err error) {
	c := newExConn(nil)
	core, err := dialWith(dial, 10*time.Second, protocol.MEx.FrameSetup, protocol.MEx.FrameCount, c.decode, op...)
	if err != nil {
		return nil, err
	}
	c.conn = core
	return c, nil
}
