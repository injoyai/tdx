package tdx

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// MarketClient 统一业务入口，每类协议最多持有一条连接；Conn 表示单条连接并实现同一业务接口。
type MarketClient struct {
	mu         sync.Mutex
	dialMu     [2]sync.Mutex
	dialing    sync.WaitGroup
	links      [2]*Conn
	dial       [2]func() (*Conn, error)
	timeout    time.Duration
	timeoutSet bool
	closed     bool
	closeDone  chan struct{}
	closeErr   error
	quoteMu    sync.Mutex
	quoteCodes *CodesBase
}

// NewClient 不立即联网；首次访问某类市场时连接，对应错误由查询方法返回。
// options 应用于两类底层连接，例如 WithRedial(false)、WithLevel(LevelNone)。
func NewClient(options ...client.Option) Client {
	return newMarketClient(options...)
}

func newMarketClient(options ...client.Option) *MarketClient {
	op := append([]client.Option(nil), options...)
	return &MarketClient{
		dial: [2]func() (*Conn, error){
			func() (*Conn, error) { return DialDefault(op...) },
			func() (*Conn, error) { return DialExHqDefault(op...) },
		},
		closeDone:  make(chan struct{}),
		quoteCodes: NewCodesBase(),
	}
}

func (c *MarketClient) connection(ex protocol.Exchange) (*Conn, error) {
	i := protocol.Instrument{Exchange: ex}
	m, err := i.Market()
	if err != nil {
		return nil, err
	}
	info, _ := m.Info()
	index := 0
	if info.Protocol == protocol.ProtocolExtended {
		index = 1
	}
	c.dialMu[index].Lock()
	defer c.dialMu[index].Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, net.ErrClosed
	}
	if link := c.links[index]; link != nil {
		c.mu.Unlock()
		return link, nil
	}
	// 注册与关闭标记共用锁，Close 开始等待后不会再新增建连任务。
	c.dialing.Add(1)
	c.mu.Unlock()
	defer c.dialing.Done()
	link, err := c.dial[index]()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		if link != nil {
			closeErr := link.CloseAll()
			c.mu.Lock()
			c.closeErr = errors.Join(c.closeErr, closeErr)
			c.mu.Unlock()
		}
		return nil, net.ErrClosed
	}
	defer c.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("%s 行情连接: %w", info.Protocol, err)
	}
	if c.timeoutSet {
		link.SetTimeout(c.timeout)
	}
	c.links[index] = link
	return link, nil
}

// SetTimeout 同时设置已有连接和后续新建连接的请求超时。
func (c *MarketClient) SetTimeout(t time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeout = t
	c.timeoutSet = true
	for _, link := range c.links {
		if link != nil {
			link.SetTimeout(t)
		}
	}
}

// Close 禁止再次建连，并等待两类底层连接关闭；可重复或并发调用。
func (c *MarketClient) Close() error {
	c.mu.Lock()
	if c.closed {
		done := c.closeDone
		c.mu.Unlock()
		<-done
		return c.closeErr
	}
	c.closed = true
	links := c.links
	c.mu.Unlock()
	var err error
	for _, link := range links {
		if link != nil {
			err = errors.Join(err, link.CloseAll())
		}
	}
	c.dialing.Wait()
	c.mu.Lock()
	c.closeErr = errors.Join(c.closeErr, err)
	close(c.closeDone)
	c.mu.Unlock()
	return c.closeErr
}

func instrumentInfo(code string) (protocol.Instrument, protocol.MarketInfo, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return i, protocol.MarketInfo{}, err
	}
	m, err := i.Market()
	if err != nil {
		return i, protocol.MarketInfo{}, err
	}
	info, _ := m.Info()
	return i, info, nil
}

func standardCode(i protocol.Instrument) string { return i.Exchange.String() + i.Code }

// GetKline 使用公开 TypeKline* 常量；日线和一分钟别名按市场配置映射。
func (c *MarketClient) GetKline(category uint8, code string, start, count uint16) (*MarketKlineResp, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	if category > protocol.TypeKlineYear || count == 0 || count > 800 {
		return nil, fmt.Errorf("K线周期必须为0..11，单次数量必须为1..800")
	}
	unit := VolumeUnknown
	switch category {
	case protocol.TypeKlineDay, protocol.TypeKlineDay2:
		category, unit = info.Day.Category, info.Day.VolumeUnit
	case protocol.TypeKlineMinute, protocol.TypeKlineMinute2:
		category, unit = info.Minute.Category, info.Minute.VolumeUnit
	default:
		if info.Protocol == protocol.ProtocolStandard {
			unit = info.Day.VolumeUnit
		}
	}
	if unit == protocol.VolumeDefault || protocol.IsConvertibleBond(standardCode(i)) {
		unit = VolumeUnknown
	}
	link, err := c.connection(i.Exchange)
	if err != nil {
		return nil, err
	}
	out := &MarketKlineResp{Instrument: i, Category: category, VolumeUnit: unit}
	if info.Protocol == protocol.ProtocolStandard {
		r, err := link.getStandardKline(category, standardCode(i), start, count, info.Kind)
		if err != nil {
			return nil, err
		}
		for _, k := range r.List {
			out.List = append(out.List, standardKline(k))
		}
	} else {
		bars, err := link.getExtendedKline(category, i.Exchange.Uint8(), i.Code, start, count)
		if err != nil {
			return nil, err
		}
		for j := range bars {
			k := &bars[j]
			out.List = append(out.List, MarketKline{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: int64(k.Trade), Extended: k})
		}
	}
	if len(out.List) > int(count) {
		return nil, errors.New("K线响应数量超过请求数量")
	}
	out.Count = len(out.List)
	return out, nil
}

// GetKlineUntil 由近到远分页，返回时间升序，包含首次命中停止条件的记录。
func (c *MarketClient) GetKlineUntil(category uint8, code string, stop func(MarketKline) bool) (*MarketKlineResp, error) {
	if stop == nil {
		return nil, errors.New("K线停止条件不能为空")
	}
	var out *MarketKlineResp
	var last *protocol.Kline
	for start := 0; start <= 65535; start += 800 {
		r, err := c.GetKline(category, code, uint16(start), 800)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = &MarketKlineResp{Instrument: r.Instrument, Category: r.Category, VolumeUnit: r.VolumeUnit}
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

func (c *MarketClient) GetKlineAll(category uint8, code string) (*MarketKlineResp, error) {
	return c.GetKlineUntil(category, code, func(MarketKline) bool { return false })
}

// GetQuote 保持输入顺序（包括重复代码）；任一请求失败返回错误，不返回部分结果。
func (c *MarketClient) GetQuote(codes ...string) ([]MarketQuote, error) {
	instruments := make([]protocol.Instrument, len(codes))
	var indexes []int
	var standard []string
	for j, code := range codes {
		i, info, err := instrumentInfo(code)
		if err != nil {
			return nil, err
		}
		instruments[j] = i
		if info.Protocol == protocol.ProtocolStandard {
			indexes = append(indexes, j)
			standard = append(standard, standardCode(i))
		}
	}
	// 2 字节通讯类型 + 10 字节固定请求体 + 每个标准品种 7 字节。
	if len(standard) > (65535-12)/7 {
		return nil, errors.New("标准报价请求超过协议 uint16 帧长上限")
	}
	out := make([]MarketQuote, len(codes))
	if len(standard) > 0 {
		link, err := c.connection(instruments[indexes[0]].Exchange)
		if err != nil {
			return nil, err
		}
		quotes, err := c.standardQuotes(link, standard)
		if err != nil {
			return nil, err
		}
		for j, q := range quotes {
			i := instruments[indexes[j]]
			if q.Exchange != i.Exchange || q.Code != i.Code {
				return nil, fmt.Errorf("标准行情响应品种与请求不一致: %s", i)
			}
			k := q.Kline
			u := MarketQuote{Instrument: i, PreClose: k.Last.Float64(), Open: k.Open.Float64(), High: k.High.Float64(), Low: k.Low.Float64(), Price: k.Close.Float64(), Volume: k.Volume, VolumeUnit: VolumeUnknown, Standard: q}
			for n := 0; n < 5; n++ {
				u.Bid[n], u.BidVolume[n] = q.BuyLevel[n].Price.Float64(), int64(q.BuyLevel[n].Number)
				u.Ask[n], u.AskVolume[n] = q.SellLevel[n].Price.Float64(), int64(q.SellLevel[n].Number)
			}
			out[indexes[j]] = u
		}
	}
	for j, i := range instruments {
		if i.Exchange == protocol.ExchangeSZ || i.Exchange == protocol.ExchangeSH || i.Exchange == protocol.ExchangeBJ {
			continue
		}
		link, err := c.connection(i.Exchange)
		if err != nil {
			return nil, err
		}
		q, err := link.getExtendedQuote(i.Exchange.Uint8(), i.Code)
		if err != nil {
			return nil, err
		}
		if q.Market != i.Exchange.Uint8() || q.Code != i.Code {
			return nil, fmt.Errorf("扩展行情响应品种与请求不一致: %s", i)
		}
		u := MarketQuote{Instrument: i, PreClose: q.PreClose, Open: q.Open, High: q.High, Low: q.Low, Price: q.Price, Volume: int64(q.ZongLiang), VolumeUnit: VolumeUnknown, Bid: q.Bid, Ask: q.Ask, Extended: q}
		for n := 0; n < 5; n++ {
			u.BidVolume[n], u.AskVolume[n] = int64(q.BidVol[n]), int64(q.AskVol[n])
		}
		out[j] = u
	}
	return out, nil
}

// GetQuoteList 按扩展市场分页查询列表，与按指定证券查询的 GetQuote 分开。
func (c *MarketClient) GetQuoteList(market, category uint8, start, count uint16) ([]protocol.ExQuoteListItem, error) {
	if market <= protocol.ExchangeBJ.Uint8() {
		return nil, errors.New("市场行情列表需要扩展市场")
	}
	link, err := c.connection(protocol.Exchange(market))
	if err != nil {
		return nil, err
	}
	return link.GetQuoteList(market, category, start, count)
}

// standardQuotes 在需要价格精度校准时取元数据，避免全局 DefaultCodes 和额外数据库/连接。
func (c *MarketClient) standardQuotes(link *Conn, codes []string) (protocol.QuotesResp, error) {
	c.quoteMu.Lock()
	defer c.quoteMu.Unlock()
	loaded := map[protocol.Exchange]bool{}
	for _, code := range codes {
		if protocol.IsStock(code) || protocol.IsIndex(code) || c.quoteCodes.Get(code) != nil {
			continue
		}
		ex, _, err := protocol.DecodeCode(code)
		if err != nil {
			return nil, err
		}
		if loaded[ex] {
			continue
		}
		r, err := link.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		var models []*CodeModel
		for _, m := range c.quoteCodes.Iter() {
			if m.Exchange != ex.String() {
				models = append(models, m)
			}
		}
		for _, m := range r.List {
			models = append(models, &CodeModel{Exchange: ex.String(), Code: m.Code, Name: m.Name, Decimal: m.Decimal, Multiple: m.Multiple, LastPrice: m.LastPrice})
		}
		c.quoteCodes.Update(models)
		loaded[ex] = true
	}
	return link.getStandardQuotes(c.quoteCodes, codes...)
}
