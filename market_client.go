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

// MarketClient 统一业务入口，分别路由到 StandardClient 和 ExtendedClient；每类协议最多一条连接。
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
		closeDone: make(chan struct{}),
	}
}

func (c *MarketClient) connection(ex protocol.Exchange) (protocolClient, error) {
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
		return link.protocolClient(), nil
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
	if link == nil || link.extended != (index == 1) {
		if link != nil {
			_ = link.CloseAll()
		}
		return nil, fmt.Errorf("%s 行情连接协议不匹配或为空", info.Protocol)
	}
	if c.timeoutSet {
		link.SetTimeout(c.timeout)
	}
	c.links[index] = link
	return link.protocolClient(), nil
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
func (c *MarketClient) GetKline(category uint8, code string, start, count uint16) (*KlineResp, error) {
	request, _, err := klineRequest(code, category, count)
	if err != nil {
		return nil, err
	}
	link, err := c.connection(request.Instrument.Exchange)
	if err != nil {
		return nil, err
	}
	return link.GetKline(category, code, start, count)
}

// GetKlineUntil 由近到远分页，返回时间升序，包含首次命中停止条件的记录。
func (c *MarketClient) GetKlineUntil(category uint8, code string, stop func(Kline) bool) (*KlineResp, error) {
	if stop == nil {
		return nil, errors.New("K线停止条件不能为空")
	}
	var out *KlineResp
	var last *protocol.Kline
	for start := 0; start <= 65535; start += 800 {
		r, err := c.GetKline(category, code, uint16(start), 800)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = &KlineResp{Instrument: r.Instrument, Category: r.Category, VolumeUnit: r.VolumeUnit}
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

func (c *MarketClient) GetKlineAll(category uint8, code string) (*KlineResp, error) {
	return c.GetKlineUntil(category, code, func(Kline) bool { return false })
}

// GetQuote 保持输入顺序（包括重复代码）；任一请求失败返回错误，不返回部分结果。
func (c *MarketClient) GetQuote(codes ...string) ([]Quote, error) {
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
	out := make([]Quote, len(codes))
	if len(standard) > 0 {
		link, err := c.connection(instruments[indexes[0]].Exchange)
		if err != nil {
			return nil, err
		}
		quotes, err := link.GetQuote(standard...)
		if err != nil {
			return nil, err
		}
		for j, q := range quotes {
			out[indexes[j]] = q
		}
	}
	for j, i := range instruments {
		if i.Exchange <= protocol.ExchangeBJ {
			continue
		}
		link, err := c.connection(i.Exchange)
		if err != nil {
			return nil, err
		}
		quotes, err := link.GetQuote(codes[j])
		if err != nil {
			return nil, err
		}
		out[j] = quotes[0]
	}
	return out, nil
}

// GetQuoteList 按扩展市场分页查询列表，与按指定证券查询的 GetQuote 分开。
func (c *MarketClient) GetQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error) {
	if market <= protocol.ExchangeBJ.Uint8() {
		return nil, errors.New("市场行情列表需要扩展市场")
	}
	link, err := c.connection(protocol.Exchange(market))
	if err != nil {
		return nil, err
	}
	return link.GetQuoteList(market, category, start, count)
}
