package tdx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// ExConn 是绑定扩展行情协议的业务连接，实现 Client；财务、F10 等标准专用查询返回不支持错误。
// DialEx* 直接返回该类型，不会建立标准连接。
type ExConn struct {
	*conn
	*queries
	requestCtx context.Context
}

// GetKline 请求扩展 K 线，保留持仓、结算价等扩展字段。
func (c *ExConn) GetKline(category uint8, code string, start, count uint16) (*KlineResp, error) {
	if err := klineBounds(category, count); err != nil {
		return nil, err
	}
	i, info, err := c.instrument(code)
	if err != nil {
		return nil, err
	}
	rule := info.Kline(category, protocol.KlinePage)
	out := &KlineResp{Instrument: i, Category: rule.Category}
	scale, err := klineVolumeScale(i, rule)
	if err != nil {
		return nil, err
	}
	page, err := c.getExtendedKline(out.Category, i.Exchange.Uint8(), i.Code, start, count)
	if err != nil {
		return nil, err
	}
	if page.count > int(count) {
		return nil, errors.New("K线响应数量超过请求数量")
	}
	for j := range page.bars {
		k := &page.bars[j]
		row := Kline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Extended: k}
		if scale > 0 {
			row.Volume, err = scaleVolume(int64(k.Trade), scale)
			if err != nil {
				return nil, err
			}
		}
		if rule.AmountField == protocol.AmountInAmount {
			amount := k.Amount * rule.AmountScale
			row.Amount = &amount
		}
		row.applyFields(rule.Fields)
		out.List = append(out.List, row)
	}
	out.Count = len(out.List)
	out.Skipped = page.count - out.Count
	return out, nil
}

// GetQuote 逐品种获取扩展五档报价，任一请求失败不返回部分结果。
func (c *ExConn) GetQuote(codes ...string) ([]Quote, error) {
	instruments := make([]protocol.Instrument, len(codes))
	quotes := make([]*protocol.ExQuote, len(codes))
	for j, code := range codes {
		i, _, quote, err := c.instrumentWithQuote(code)
		if err != nil {
			return nil, err
		}
		if _, err := volumeScale(i); err != nil {
			return nil, err
		}
		instruments[j], quotes[j] = i, quote
	}
	out := make([]Quote, len(codes))
	for j, i := range instruments {
		q := quotes[j]
		if q == nil {
			var err error
			q, err = c.instrumentQuote(i)
			if err != nil {
				return nil, err
			}
		}
		u := Quote{Instrument: i, PreClose: q.PreClose, Open: q.Open, High: q.High, Low: q.Low, Price: q.Price, Volume: int64(q.ZongLiang), Bid: q.Bid, Ask: q.Ask, Extended: q}
		for n := 0; n < 5; n++ {
			u.BidVolume[n], u.AskVolume[n] = int64(q.BidVol[n]), int64(q.AskVol[n])
		}
		info, _ := i.Rules()
		if err := u.applyRule(info.Quote); err != nil {
			return nil, err
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
	items, err := c.GetInstruments()
	if err != nil {
		return nil, err
	}
	out := &CodeResp{List: []Code{}}
	for j := range items {
		k := &items[j]
		if k.Market == ex.Uint8() {
			out.List = append(out.List, Code{Instrument: protocol.Instrument{Exchange: ex, Code: k.Code}, Name: k.Name, Extended: k})
		}
	}
	out.Count = len(out.List)
	return out, nil
}

const instrumentPageSize = 800

var errInstrumentConnectionChanged = fmt.Errorf("扩展品种扫描期间连接已更换: %w", io.ErrUnexpectedEOF)

// GetInstruments 读取扩展服务器可分页提供的全局品种表。
// 整次扫描使用同一连接。报告总数与实读数的尾部差异经复查后允许返回。
func (c *ExConn) GetInstruments() ([]protocol.ExInstrument, error) {
	writer := c.currentWriter()
	items, err := c.scanInstruments()
	if err != nil {
		return nil, err
	}
	if c.currentWriter() != writer {
		return nil, errors.New("扩展行情连接在品种扫描期间变更")
	}
	if err := c.requestContext().Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (c *ExConn) instrument(code string) (protocol.Instrument, protocol.InstrumentRules, error) {
	i, rules, _, err := c.instrumentWithQuote(code)
	return i, rules, err
}

func (c *ExConn) instrumentWithQuote(code string) (protocol.Instrument, protocol.InstrumentRules, *protocol.ExQuote, error) {
	i, err := protocol.ResolveInstrument(code, nil)
	var quote *protocol.ExQuote
	if err != nil {
		if lookup, ok := protocol.LookupInstrument(code); ok {
			i, quote, err = c.probeInstrument(lookup)
		}
	}
	if err != nil {
		return i, protocol.InstrumentRules{}, nil, err
	}
	info, ok := i.Rules()
	if !ok || info.Protocol != protocol.ProtocolExtended {
		return i, info, nil, protocolMismatch()
	}
	return i, info, quote, nil
}

func (c *ExConn) probeInstrument(lookup protocol.InstrumentLookup) (protocol.Instrument, *protocol.ExQuote, error) {
	var found protocol.Instrument
	var quote *protocol.ExQuote
	writer := c.currentWriter()
	for _, exchange := range lookup.Exchanges {
		i := protocol.Instrument{Exchange: exchange, Code: lookup.Code}
		q, err := c.instrumentQuote(i)
		if err != nil {
			return protocol.Instrument{}, nil, fmt.Errorf("%s 无法确认候选市场 %d: %w", lookup, exchange, err)
		}
		if c.currentWriter() != writer {
			return protocol.Instrument{}, nil, fmt.Errorf("%s 扩展行情连接在市场确认期间变更", lookup)
		}
		// 缺少行情的候选市场可能回显请求身份，其余字段全部为零。
		if *q == (protocol.ExQuote{Market: exchange.Uint8(), Code: lookup.Code}) {
			continue
		}
		if quote != nil {
			return protocol.Instrument{}, nil, fmt.Errorf("%s 同时在通达信市场 %d 和 %d 返回有效报价", lookup, found.Exchange, exchange)
		}
		found, quote = i, q
	}
	if quote == nil {
		return protocol.Instrument{}, nil, fmt.Errorf("%s 未取得可确认市场的报价（候选市场 %s）", lookup, lookup.Candidates())
	}
	return found, quote, nil
}

func (c *ExConn) scanInstruments() ([]protocol.ExInstrument, error) {
	writer := c.currentWriter()
	total, err := c.getExtendedCount()
	if err != nil {
		return nil, err
	}
	if c.currentWriter() != writer {
		return nil, errInstrumentConnectionChanged
	}
	out := []protocol.ExInstrument{}
	shortTail := false
	for start := 0; start < total; {
		count := min(instrumentPageSize, total-start)
		items, err := c.getExtendedInstruments(uint32(start), uint16(count))
		if err != nil {
			return nil, err
		}
		if c.currentWriter() != writer {
			return nil, errInstrumentConnectionChanged
		}
		if len(items) == 0 && shortTail {
			confirmed, err := c.confirmInstrumentEnd(start, total, out[start-1])
			if err != nil {
				return nil, err
			}
			if c.currentWriter() != writer {
				return nil, errInstrumentConnectionChanged
			}
			if confirmed {
				c.logger().Warn("扩展品种报告总数高于可读条数", "server", writer.server, "reported_total", total, "read_total", start)
				return out, nil
			}
		}
		if len(items) == 0 || len(items) > count {
			return nil, fmt.Errorf("扩展品种分页 %d 不完整: %w", start, io.ErrUnexpectedEOF)
		}
		shortTail = count == total-start && len(items) < count
		out = append(out, items...)
		start += len(items)
	}
	return out, nil
}

func (c *ExConn) confirmInstrumentEnd(start, total int, last protocol.ExInstrument) (bool, error) {
	anchor, err := c.getExtendedInstruments(uint32(start-1), 1)
	if err != nil || len(anchor) != 1 || anchor[0].Market != last.Market || anchor[0].Code != last.Code {
		return false, err
	}
	// 用常规页长复查，避免把与请求条数有关的空响应识别为表尾。
	rows, err := c.getExtendedInstruments(uint32(start), instrumentPageSize)
	if err != nil || len(rows) != 0 {
		return false, err
	}
	rows, err = c.getExtendedInstruments(uint32(total-1), 1)
	if err != nil || len(rows) != 0 {
		return false, err
	}
	count, err := c.getExtendedCount()
	return err == nil && count == total, err
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
	i, info, err := c.instrument(code)
	if err != nil {
		return nil, err
	}
	rule := info.TimeShare
	if err := checkTickRule(i, "分时", rule); err != nil {
		return nil, err
	}
	out := &MinuteResp{Instrument: i, Date: date}
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
		volume, amount, err := tickVolumeAmount(int64(k.Volume), rule)
		if err != nil {
			return nil, err
		}
		var price float64
		if rule.Fields.Price {
			price = k.Price / rule.PriceDivisor
		}
		out.List = append(out.List, Minute{Time: fmt.Sprintf("%02d:%02d", k.Hour, k.Minute), Price: price, Volume: volume, Amount: amount, Extended: k})
	}
	out.Count = len(out.List)
	return out, nil
}

func (c *ExConn) tradePage(date, code string, start, count uint16) (*TradeResp, error) {
	i, info, err := c.instrument(code)
	if err != nil {
		return nil, err
	}
	rule := info.Trade
	if err := checkTickRule(i, "分笔", rule); err != nil {
		return nil, err
	}
	out := &TradeResp{Instrument: i, Date: date}
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
	for j := range ticks {
		k := &ticks[j]
		volume, amount, err := tickVolumeAmount(int64(k.Volume), rule)
		if err != nil {
			return nil, err
		}
		var price *float64
		if value, ok := info.ExtendedTradePrice(k.Price); ok {
			price = &value
		}
		out.List = append(out.List, Trade{Date: date, Time: fmt.Sprintf("%02d:%02d:%02d", k.Hour, k.Minute, k.Second), Price: price, Volume: volume, Amount: amount, Direction: k.Direction, Extended: k})
	}
	out.Count = len(out.List)
	return out, nil
}

// GetQuoteList 获取扩展市场分页报价，保留原始字段和单位。
func (c *ExConn) GetQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error) {
	if _, err := quoteListRoute(market, category); err != nil {
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
	i, info, err := c.instrument(code)
	if err != nil {
		return nil, err
	}
	rule := info.Kline(protocol.TypeKlineMinute, protocol.KlineRange)
	scale, err := klineVolumeScale(i, rule)
	if err != nil {
		return nil, err
	}
	r, err := c.getExtendedKlineRange(i.Exchange.Uint8(), i.Code, start, end)
	if err != nil {
		return nil, err
	}
	out := &KlineResp{Instrument: i, Category: protocol.TypeKlineMinute}
	for j := range r {
		k := &r[j]
		row := Kline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, RangeExtended: k}
		if scale > 0 {
			row.Volume, err = scaleVolume(int64(k.Trade), scale)
			if err != nil {
				return nil, err
			}
		}
		if rule.AmountField == protocol.AmountInPosition {
			amount := float64(math.Float32frombits(k.Position)) * rule.AmountScale
			row.Amount = &amount
		}
		row.applyFields(rule.Fields)
		out.List = append(out.List, row)
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
	r, err := c.getExtendedMarkets()
	if err != nil {
		return nil, err
	}
	out := make([]Exchange, 0, len(r))
	for j := range r {
		m := &r[j]
		shortName := m.ShortName
		ex := protocol.Exchange(m.Market)
		if _, err := (protocol.Instrument{Exchange: ex}).Market(); err == nil {
			shortName = ex.String()
		}
		out = append(out, Exchange{Market: m.Market, Name: m.Name, ShortName: shortName, Protocol: protocol.ProtocolExtended, Extended: m})
	}
	return out, nil
}

// getExtendedMarkets 市场代码表。
func (c *ExConn) getExtendedMarkets() ([]protocol.ExMarket, error) {
	r, err := c.SendFrame(protocol.MEx.FrameMarkets())
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMarket), nil
}

// getExtendedCount 全部品种数量。
func (c *ExConn) getExtendedCount() (int, error) {
	r, err := c.SendFrame(protocol.MEx.FrameCount())
	if err != nil {
		return 0, err
	}
	return r.(int), nil
}

// getExtendedInstruments 按全局序号分页的品种列表。
func (c *ExConn) getExtendedInstruments(start uint32, count uint16) ([]protocol.ExInstrument, error) {
	r, err := c.SendFrame(protocol.MEx.FrameInstrument(start, count))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExInstrument), nil
}

func (c *ExConn) instrumentQuote(i protocol.Instrument) (*protocol.ExQuote, error) {
	q, err := c.getExtendedQuote(i.Exchange.Uint8(), i.Code)
	if err != nil {
		return nil, err
	}
	if q.Market != i.Exchange.Uint8() || q.Code != i.Code {
		return nil, fmt.Errorf("扩展行情响应品种与请求不一致: %s", i)
	}
	return q, nil
}

// getExtendedQuote 单品种五档行情。
func (c *ExConn) getExtendedQuote(market uint8, code string) (*protocol.ExQuote, error) {
	r, err := c.SendFrame(protocol.MEx.FrameQuote(market, code))
	if err != nil {
		return nil, err
	}
	return r.(*protocol.ExQuote), nil
}

// GetQuoteList 批量行情列表(category 2=港股 3=期货)。
func (c *ExConn) getQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error) {
	r, err := c.SendFrame(protocol.MEx.FrameQuoteList(market, start, count), protocol.ExQuoteListCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.QuoteListItem), nil
}

type exBarsPageCache protocol.ExBarsCache

type exBarsPage struct {
	bars  []protocol.ExKline
	count int
}

// getExtendedKline K线(category 同标准 KLINE 类型)。
func (c *ExConn) getExtendedKline(category, market uint8, code string, start, count uint16) (exBarsPage, error) {
	r, err := c.SendFrame(protocol.MEx.FrameBars(category, market, code, start, count), exBarsPageCache{Category: category})
	if err != nil {
		return exBarsPage{}, err
	}
	return r.(exBarsPage), nil
}

// getExtendedMinute 当日分时。
func (c *ExConn) getExtendedMinute(market uint8, code string) ([]protocol.ExMinuteTick, error) {
	r, err := c.SendFrame(protocol.MEx.FrameMinute(market, code))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// getExtendedHistoryMinute 历史分时(date=YYYYMMDD)。
func (c *ExConn) getExtendedHistoryMinute(market uint8, code string, date uint32) ([]protocol.ExMinuteTick, error) {
	r, err := c.SendFrame(protocol.MEx.FrameHistMinute(market, code, date))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// getExtendedTrade 当日分笔成交。
func (c *ExConn) getExtendedTrade(market uint8, code string, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := c.SendFrame(protocol.MEx.FrameTrade(market, code, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// getExtendedHistoryTrade 历史分笔成交(date=YYYYMMDD)。
func (c *ExConn) getExtendedHistoryTrade(market uint8, code string, date uint32, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := c.SendFrame(protocol.MEx.FrameHistTrade(market, code, date, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// getExtendedKlineRange 历史K线区间(date/date2=YYYYMMDD)。
func (c *ExConn) getExtendedKlineRange(market uint8, code string, date, date2 uint32) ([]protocol.ExRangeKline, error) {
	r, err := c.SendFrame(protocol.MEx.FrameBarsRange(market, code, date, date2))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExRangeKline), nil
}
func (*ExConn) decode(f *protocol.Response, val any) (resp any, err error) {
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
		if cache, ok := val.(exBarsPageCache); ok {
			bars, count, err := protocol.MEx.DecodeBarsPage(f.Data, protocol.ExBarsCache(cache))
			return exBarsPage{bars: bars, count: count}, err
		}
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
func (*ExConn) protocol() protocol.MarketProtocol { return protocol.ProtocolExtended }

func (*ExConn) GetBlockData(file string) ([]*protocol.Block, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetBlockFileRaw(file string) ([]byte, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetCompanyCategory(code string) ([]protocol.CompanyCategory, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetCompanyContent(code, filename string, start, length uint32) (string, error) {
	return "", errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetETFCodeAll() ([]string, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetFinanceInfo(code string) (*protocol.FinanceInfo, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetGbbq(code string) (*protocol.GbbqResp, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetIndexCodeAll() ([]string, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetReportFile(file string) ([]byte, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetSpBlock() ([]*protocol.SpBlock, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetStockCodeAll() ([]string, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetTdxBk() ([]*protocol.TdxBk, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetTdxHy() ([]*protocol.TdxHy, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetTdxStat() ([]*protocol.TdxStat, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetTdxZs() ([]*protocol.TdxZs, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetXgsg() ([]*protocol.TdxXgsg, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

func (*ExConn) GetZHBFiles() (map[string][]byte, error) {
	return nil, errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
}

// DialExDefault 连接默认扩展行情服务器(轮询 ExHosts)。
func DialExDefault(op ...client.Option) (*ExConn, error) {
	op = append([]client.Option{WithRedial()}, op...)
	return DialExHosts(ExHosts, op...)
}

// DialEx 连接指定扩展行情服务器。
func DialEx(addr string, op ...client.Option) (*ExConn, error) {
	return dialExWith(NewTCPDial(withPort(addr, ExPort)), op...)
}

// DialExWith 通过自定义拨号函数建立扩展行情连接，供离线模拟报文使用。
func DialExWith(dial ios.DialFunc, op ...client.Option) (*ExConn, error) {
	return dialExWith(dial, op...)
}

// DialExHosts 遍历扩展行情服务器，裸 IP 默认使用端口 7727。
func DialExHosts(hosts []string, op ...client.Option) (*ExConn, error) {
	return dialExWith(NewRangeDial(hostAddrs(hosts, ExHosts, ExPort)), op...)
}

// dialExWith 建立扩展行情连接:握手用 ExSetup,心跳用品种数量请求。
func dialExWith(dial ios.DialFunc, op ...client.Option) (cli *ExConn, err error) {
	return dialEx(context.Background(), dial, op...)
}

func dialEx(ctx context.Context, dial ios.DialFunc, op ...client.Option) (*ExConn, error) {
	return dialExContexts(ctx, ctx, dial, op...)
}

func dialExContexts(lifecycle, initial context.Context, dial ios.DialFunc, op ...client.Option) (*ExConn, error) {
	c := newExConn(nil)
	core, err := dialWithContexts(lifecycle, initial, dial, 10*time.Second, protocol.MEx.FrameSetup, protocol.MEx.FrameCount, c.decode, op...)
	if err != nil {
		return nil, err
	}
	c.conn = core
	return c, nil
}
