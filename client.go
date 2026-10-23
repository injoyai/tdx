package tdx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// 统一结果的价格使用品种原币种/报价单位，浮点值保留底层解码精度。
// VolumeUnit 只描述对应接口；unknown 表示保留解码数值且不做推测换算。
const VolumeUnknown protocol.VolumeUnit = "unknown"

// KlineResp 是统一 K 线结果，Category 为实际协议周期，List 按时间升序排列。
type KlineResp struct {
	Instrument protocol.Instrument
	Category   uint8 // 实际发送的协议周期
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []Kline
}

// Kline 是统一 K 线记录，Standard/Extended/RangeExtended 按查询来源保留原始字段。
type Kline struct {
	Time          string // 源数据的日期/墙上时间 YYYY-MM-DD HH:MM；不推断时区或夜盘交易日
	Open          float64
	High          float64
	Low           float64
	Close         float64
	Volume        int64
	Amount        *float64 // nil 表示该市场的金额语义未确认
	Standard      *protocol.Kline
	Extended      *protocol.ExKline
	RangeExtended *protocol.ExRangeKline // 区间协议的持仓和结算价等原始字段
}

// Quote 是统一五档报价，价格使用品种原币种/报价单位，成交量以 VolumeUnit 为准。
type Quote struct {
	Instrument protocol.Instrument
	PreClose   float64
	Open       float64
	High       float64
	Low        float64
	Price      float64
	Volume     int64
	VolumeUnit protocol.VolumeUnit
	Bid        [5]float64
	BidVolume  [5]int64
	Ask        [5]float64
	AskVolume  [5]int64
	Standard   *protocol.Quote
	Extended   *protocol.ExQuote
}

// MinuteResp 是统一分时结果，Date 保留历史查询日期，不推断夜盘日期。
type MinuteResp struct {
	Instrument protocol.Instrument
	Date       string // 请求日期 YYYYMMDD，不按行号推断夜盘日期
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []Minute
}

// Minute 是统一分时记录，Standard 或 Extended 保留对应协议的原始数据。
type Minute struct {
	Time     string // HH:MM
	Price    float64
	Volume   int64
	Standard *protocol.PriceNumber
	Extended *protocol.ExMinuteTick
}

// TradeResp 是统一分笔结果；跨日查询的 Date 为空，交易日期保留在每条记录中。
type TradeResp struct {
	Instrument protocol.Instrument
	Date       string
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []Trade
}

// Trade 是统一分笔记录；价格尺度未确认时 Price 为 nil，原始值仍可从 Extended 读取。
type Trade struct {
	Date      string   // 历史查询日期 YYYYMMDD；跨日结果逐条保留，不推断当日扩展日期
	Time      string   // HH:MM:SS，源数据无秒时为 00
	Price     *float64 // 扩展分笔原始整数价格尺度未确认，保留 Extended.Price，公共价格为 nil
	Volume    int64
	Direction int // 1 买，-1 卖，0 中性
	Standard  *protocol.Trade
	Extended  *protocol.ExTradeTick
}

// Code 是指定市场的品种信息，价格精度等协议特有字段保留在 Standard 或 Extended 中。
type Code struct {
	Instrument protocol.Instrument
	Name       string
	Standard   *protocol.Code
	Extended   *protocol.ExInstrument
}

// CodeResp 是统一代码列表结果，Count 为本次返回数量，不是市场品种总数。
type CodeResp struct {
	Count int
	List  []Code
}

// Exchange 保留服务器市场编号；发现市场不表示统一行情路由已接入。
type Exchange struct {
	Market    uint16
	Name      string
	ShortName string
	Protocol  protocol.MarketProtocol
	Extended  *protocol.ExMarket
}

// MarketClient 统一业务入口，分别路由到 StConn 和 ExConn；每类协议最多一条连接。
type MarketClient struct {
	*queries
	mu         sync.Mutex
	dialMu     [2]sync.Mutex
	dialing    sync.WaitGroup
	links      [2]protocolClient
	dial       [2]func() (protocolClient, error)
	cancel     context.CancelFunc
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
	ctx, cancel := context.WithCancel(context.Background())
	c := &MarketClient{
		cancel: cancel,
		dial: [2]func() (protocolClient, error){
			func() (protocolClient, error) {
				return dialSt(ctx, NewRangeDial(hostAddrs(Hosts, Hosts, stPort)), append([]client.Option{WithRedial()}, op...)...)
			},
			func() (protocolClient, error) {
				return dialEx(ctx, NewRangeDial(hostAddrs(ExHosts, ExHosts, ExPort)), append([]client.Option{WithRedial()}, op...)...)
			},
		},
		closeDone: make(chan struct{}),
	}
	c.queries = &queries{source: c}
	return c
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
		if clientOrNil(link) != nil {
			closeErr := link.Close()
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
	if clientOrNil(link) == nil || (link.protocol() == protocol.ProtocolExtended) != (index == 1) {
		if clientOrNil(link) != nil {
			_ = link.Close()
		}
		return nil, fmt.Errorf("%s 行情连接协议不匹配或为空", info.Protocol)
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
	c.cancel()
	links := c.links
	c.mu.Unlock()
	var err error
	for _, link := range links {
		if link != nil {
			err = errors.Join(err, link.Close())
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

func (c *MarketClient) minutePage(date, code string) (*MinuteResp, error) {
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

func (c *MarketClient) tradePage(date, code string, start, count uint16) (*TradeResp, error) {
	i, _, err := instrumentInfo(code)
	if err != nil {
		return nil, err
	}
	link, err := c.connection(i.Exchange)
	if err != nil {
		return nil, err
	}
	return link.tradePage(date, code, start, count)
}

// GetCode 的 start 是指定市场内的偏移；标准协议每次最多返回 1000 项。
func (c *MarketClient) GetCode(ex protocol.Exchange, start uint32, count uint16) (*CodeResp, error) {
	if err := codePageBounds(ex, start, count); err != nil {
		return nil, err
	}
	link, err := c.connection(ex)
	if err != nil {
		return nil, err
	}
	return link.GetCode(ex, start, count)
}

// GetCodeAll 返回指定市场的全部品种，协议扫描由对应客户端完成。
func (c *MarketClient) GetCodeAll(ex protocol.Exchange) (*CodeResp, error) {
	link, err := c.connection(ex)
	if err != nil {
		return nil, err
	}
	return link.GetCodeAll(ex)
}

// GetCount 返回指定市场数量，扩展协议全局计数不能直接替代单市场数量。
func (c *MarketClient) GetCount(ex protocol.Exchange) (int, error) {
	link, err := c.connection(ex)
	if err != nil {
		return 0, err
	}
	return link.GetCount(ex)
}

// GetMarkets 合并内置标准市场和扩展服务器市场表；扩展发现失败时返回错误。
func (c *MarketClient) GetMarkets() ([]Exchange, error) {
	link, err := c.connection(protocol.ExchangeHK)
	if err != nil {
		return nil, err
	}
	r, err := link.GetMarkets()
	if err != nil {
		return nil, err
	}
	return append(standardMarkets(), r...), nil
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

func (c *MarketClient) GetBlockData(file string) ([]*protocol.Block, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetBlockData(file)
}

func (c *MarketClient) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetBlockDataWithIndex(file)
}

func (c *MarketClient) GetBlockFileRaw(file string) ([]byte, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetBlockFileRaw(file)
}

func (c *MarketClient) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	if _, err := standardInstrument(code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetCallAuction(code)
}

func (c *MarketClient) GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error) {
	if _, err := standardSecurity(exchange, code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetCompanyCategory(exchange, code)
}

func (c *MarketClient) GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error) {
	if _, err := standardSecurity(exchange, code); err != nil {
		return "", err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return "", err
	}
	return link.GetCompanyContent(exchange, code, filename, start, length)
}

func (c *MarketClient) GetETFCodeAll() ([]string, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetETFCodeAll()
}

func (c *MarketClient) GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error) {
	if _, err := standardSecurity(exchange, code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetFinanceInfo(exchange, code)
}

func (c *MarketClient) GetGbbq(code string) (*protocol.GbbqResp, error) {
	if _, err := standardInstrument(code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetGbbq(code)
}

func (c *MarketClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetGbbqAll()
}

func (c *MarketClient) GetIndexCodeAll() ([]string, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetIndexCodeAll()
}

func (c *MarketClient) GetReportFile(file string) ([]byte, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetReportFile(file)
}

func (c *MarketClient) GetSpBlock() ([]*protocol.SpBlock, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetSpBlock()
}

func (c *MarketClient) GetStockCodeAll() ([]string, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetStockCodeAll()
}

func (c *MarketClient) GetTdxBk() ([]*protocol.TdxBk, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxBk()
}

func (c *MarketClient) GetTdxHy() ([]*protocol.TdxHy, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxHy()
}

func (c *MarketClient) GetTdxStat() ([]*protocol.TdxStat, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxStat()
}

func (c *MarketClient) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxStat2()
}

func (c *MarketClient) GetTdxZs() ([]*protocol.TdxZs, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxZs()
}

func (c *MarketClient) GetXgsg() ([]*protocol.TdxXgsg, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetXgsg()
}

func (c *MarketClient) GetZHBFiles() (map[string][]byte, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetZHBFiles()
}

func standardKline(k *protocol.Kline) Kline {
	amount := k.Amount.Float64()
	return Kline{Time: k.Time.Format("2006-01-02 15:04"), Open: k.Open.Float64(), High: k.High.Float64(), Low: k.Low.Float64(), Close: k.Close.Float64(), Volume: k.Volume, Amount: &amount, Standard: k}
}

// Klines 复制标准 K 线并统一成交量为整数手，供指标和本地文件编码使用。
// 未知单位或不能无损换算的股数返回错误；不修改原始协议记录。
func (r *KlineResp) Klines() (protocol.Klines, error) {
	raw, err := r.StandardResponse()
	if err != nil {
		return nil, err
	}
	if len(raw.List) > 0 && r.VolumeUnit != protocol.VolumeHundredShares && r.VolumeUnit != protocol.VolumeShares {
		return nil, fmt.Errorf("成交量单位 %q 不能转换为整数手", r.VolumeUnit)
	}
	out := make(protocol.Klines, 0, len(raw.List))
	for _, k := range raw.List {
		copy := *k
		if r.VolumeUnit == protocol.VolumeShares {
			if k.Volume%protocol.SharesPerLot != 0 {
				return nil, errors.New("成交量股数不能无损转换为整数手")
			}
			copy.Volume = protocol.FromShares(k.Volume)
		}
		out = append(out, &copy)
	}
	return out, nil
}

// StandardResponse 保留原始记录及其单位，供复权和旧 HTTP 数据契约使用。
// 指标与本地文件编码需要整数手，使用 Klines；指数原始量为股。
func (r *KlineResp) StandardResponse() (*protocol.KlineResp, error) {
	if r.Instrument.Exchange > protocol.ExchangeBJ {
		return nil, errors.New("扩展 K线不能转换为标准协议数据")
	}
	if len(r.List) > 65535 {
		return nil, errors.New("累计记录数超过标准返回类型 uint16 上限")
	}
	out := &protocol.KlineResp{Count: uint16(len(r.List))}
	for _, k := range r.List {
		if k.Standard == nil {
			return nil, errors.New("缺少标准 K线原始数据")
		}
		out.List = append(out.List, k.Standard)
	}
	return out, nil
}

// StandardResponse 转换标准分时结果，保留原始数据并校验记录数与来源。
func (r *MinuteResp) StandardResponse() (*protocol.MinuteResp, error) {
	if r.Instrument.Exchange > protocol.ExchangeBJ {
		return nil, errors.New("扩展分时不能转换为标准协议数据")
	}
	if len(r.List) > 65535 {
		return nil, errors.New("累计记录数超过标准返回类型 uint16 上限")
	}
	out := &protocol.MinuteResp{Count: uint16(len(r.List))}
	for _, k := range r.List {
		if k.Standard == nil {
			return nil, errors.New("缺少标准分时原始数据")
		}
		out.List = append(out.List, *k.Standard)
	}
	return out, nil
}

func (r *TradeResp) StandardResponse() (*protocol.TradeResp, error) {
	if r.Instrument.Exchange > protocol.ExchangeBJ {
		return nil, errors.New("扩展分笔不能转换为标准协议数据")
	}
	if len(r.List) > 65535 {
		return nil, errors.New("累计记录数超过标准返回类型 uint16 上限")
	}
	out := &protocol.TradeResp{Count: uint16(len(r.List))}
	for _, k := range r.List {
		if k.Standard == nil {
			return nil, errors.New("缺少标准分笔原始数据")
		}
		out.List = append(out.List, k.Standard)
	}
	return out, nil
}
