package tdx

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/module/common"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/protocol"
)

const (
	LevelNone  = common.LevelNone
	LevelDebug = common.LevelDebug
	LevelWrite = common.LevelWrite
	LevelRead  = common.LevelRead
	LevelInfo  = common.LevelInfo
	LevelError = common.LevelError
	LevelAll   = common.LevelAll
)

// WithDebug 是否打印通讯数据
func WithDebug(b ...bool) client.Option {
	return func(c *client.Client) {
		c.Logger.Debug(b...)
	}
}

func WithLevel(level int) client.Option {
	return func(c *client.Client) {
		c.Logger.SetLevel(level)
	}
}

// WithRedial 断线重连
func WithRedial(b ...bool) client.Option {
	return func(c *client.Client) {
		c.SetRedial(b...)
	}
}

// DialDefault 默认连接方式
func DialDefault(op ...client.Option) (cli *Client, err error) {
	op = append([]client.Option{WithRedial()}, op...)
	return DialHostsRange(Hosts, op...)
}

// Dial 与服务器建立连接
func Dial(addr string, op ...client.Option) (cli *Client, err error) {
	return DialWith(NewTCPDial(addr), op...)
}

// DialHosts 与服务器建立连接,多个服务器轮询,开启重试生效
func DialHosts(hosts []string, op ...client.Option) (cli *Client, err error) {
	return DialWith(NewHostDial(hosts), op...)
}

// DialHostsRandom 与服务器建立连接,多个服务器随机连接
func DialHostsRandom(hosts []string, op ...client.Option) (cli *Client, err error) {
	return DialWith(NewRandomDial(hosts), op...)
}

// DialHostsRange 遍历设置的服务地址进行连接,成功则结束遍历
func DialHostsRange(hosts []string, op ...client.Option) (cli *Client, err error) {
	return DialWith(NewRangeDial(hosts), op...)
}

// DialWith 与服务器建立连接
func DialWith(dial ios.DialFunc, op ...client.Option) (cli *Client, err error) {
	return dialWith(dial, false, op...)
}

// dialWith 两类行情共用连接生命周期，仅握手、心跳与默认超时不同。
func dialWith(dial ios.DialFunc, extended bool, op ...client.Option) (cli *Client, err error) {
	timeout := 2 * time.Second
	setup, heartbeat := protocol.MConnect.Frame, protocol.MHeart.Frame
	if extended {
		timeout = 10 * time.Second
		setup, heartbeat = protocol.MEx.FrameSetup, protocol.MEx.FrameCount
	}
	cli = &Client{
		Wait:        wait.New(timeout),
		waitTimeout: timeout,
		extended:    extended,
		m:           maps.NewSafe(),
	}

	cli.Client, err = client.Dial(cli.cancelableDial(dial), func(c *client.Client) {
		c.Logger.Debug(true)                           //关闭日志打印
		c.Logger.SetLevel(LevelInfo)                   //设置日志级别
		c.Logger.WithHEX()                             //以HEX显示
		c.SetOption(op...)                             //自定义选项
		c.Event.OnReadFrom = protocol.ReadFrom         //分包
		c.Event.OnDealMessage = cli.handlerDealMessage //解析数据并处理
		c.Event.OnConnected = func(c *client.Client) error {
			w := cli.wrapWriter(c)
			<-cli.writeGate // 连接初始化完成，允许写入。
			if _, err := cli.Write(setup().Bytes()); err != nil {
				if extended {
					c.Close()
				}
				return err
			}
			cli.goHeartbeat(w, c.Closer.Done(), heartbeat().Bytes())
			return nil
		}
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cli.cancelRun = cancel
	go cli.Client.Run(ctx)

	return cli, nil
}

type Client struct {
	extended       bool
	queryOnce      sync.Once
	queries        *MarketClient
	*client.Client              //客户端实例
	ios.MoreWriter              // 稳定写入口，重连只替换内部连接快照。
	Wait           *wait.Entity //内部响应关联；请求超时通过 SetTimeout 配置
	m              *maps.Safe   //有部分解析需要用到代码,返回数据获取不到,固请求的时候缓存下
	msgID          uint32       //消息id,使用SendFrame自动累加
	waitMu         sync.Mutex
	waitTimeout    time.Duration
	writer         *frameWriter
	writeGate      chan struct{}
	cancelRun      context.CancelFunc
}

// CloseAll 取消整个连接生命周期，避免并发修改底层库的重连开关。
func (c *Client) CloseAll() error {
	if c.cancelRun != nil {
		c.cancelRun()
		c.waitMu.Lock()
		w := c.writer
		c.waitMu.Unlock()
		if w != nil && w.conn != nil {
			_ = w.conn.Close() // 解除读取阻塞，由读取协程结束当次连接。
		}
		<-c.Client.Done()
	}
	return c.Client.Close()
}

// handlerDealMessage 处理服务器响应的数据
func (this *Client) handlerDealMessage(c *client.Client, msg ios.Acker) {

	defer func() {
		if e := recover(); e != nil {
			logs.Err(e)
			debug.PrintStack()
		}
	}()

	payload := msg.Payload()
	f, err := protocol.Decode(payload)
	if err != nil {
		logs.Err(err)
		if len(payload) >= 16 {
			key := conv.String(protocol.Uint32(payload[5:9]))
			this.waitMu.Lock()
			this.Wait.Done(key, nil, err)
			this.m.GetAndDel(key)
			this.waitMu.Unlock()
		}
		return
	}
	this.waitMu.Lock()
	defer this.waitMu.Unlock()
	if f.MsgID != 0 && !this.Wait.IsWait(conv.String(f.MsgID)) {
		return // 超时或写入失败的请求已清理，忽略迟到响应。
	}

	//从缓存中获取数据,响应数据中不同类型有不同的处理方式,但是响应无返回该类型,固根据消息id进行缓存
	val, _ := this.m.GetAndDel(conv.String(f.MsgID))

	var resp any
	switch f.Type {

	case protocol.TypeConnect:

	case protocol.TypeHeart:

	case protocol.TypeCount:
		resp, err = protocol.MCount.Decode(f.Data)

	case protocol.TypeCode:
		resp, err = protocol.MCode.Decode(f.Data)

	case protocol.TypeQuote:
		resp, err = protocol.MQuote.DecodeChecked(f.Data)

	case protocol.TypeMinute:
		resp, err = protocol.MMinute.Decode(f.Data)

	case protocol.TypeHistoryMinute:
		resp, err = protocol.MHistoryMinute.Decode(f.Data)

	case protocol.TypeCallAuction:
		resp, err = protocol.MCallAuction.Decode(f.Data)

	case protocol.TypeTrade:
		resp, err = protocol.MTrade.Decode(f.Data, val.(protocol.TradeCache))

	case protocol.TypeHistoryTrade:
		resp, err = protocol.MHistoryTrade.Decode(f.Data, val.(protocol.TradeCache))

	case protocol.TypeKline:
		resp, err = protocol.MKline.Decode(f.Data, val.(protocol.KlineCache))

	case protocol.TypeGbbq:
		resp, err = protocol.MGbbq.Decode(f.Data)

	case protocol.TypeBlockMeta:
		resp, err = protocol.MBlock.DecodeMeta(f.Data)

	case protocol.TypeBlockInfo:
		resp, err = protocol.MBlock.DecodeInfo(f.Data)

	case protocol.TypeFinance:
		resp, err = protocol.MFinance.Decode(f.Data)

	case protocol.TypeCompanyCat:
		resp, err = protocol.MCompanyCat.Decode(f.Data)

	case protocol.TypeCompanyContent:
		resp, err = protocol.MCompanyContent.Decode(f.Data)

	// ---- 扩展行情(TdxExHq) ----
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

	if err != nil {
		logs.Err(err)
		this.Wait.Done(conv.String(f.MsgID), nil, err)
		return
	}

	this.Wait.Done(conv.String(f.MsgID), resp)

}

// SetTimeout 设置超时时间
func (this *Client) SetTimeout(t time.Duration) {
	this.waitMu.Lock()
	defer this.waitMu.Unlock()
	this.waitTimeout = t
	this.Wait.SetTimeout(t)
}

// SendFrame 发送数据,并等待响应
func (this *Client) SendFrame(f *protocol.Frame, cache ...any) (any, error) {
	f.MsgID = atomic.AddUint32(&this.msgID, 1)
	key := conv.String(f.MsgID)
	if len(cache) > 0 {
		this.m.Set(key, cache[0])
	}
	type result struct {
		value any
		err   error
	}
	done := make(chan result, 1)
	// 先注册完整回调再发送；注册和响应处理互斥，避免依赖内部的 list 竞争。
	this.waitMu.Lock()
	timeout := this.waitTimeout
	w := this.writer
	this.Wait.Async(key, func(value any, err error) { done <- result{value, err} }, 1, 0)
	this.waitMu.Unlock()
	defer func() {
		this.waitMu.Lock()
		defer this.waitMu.Unlock()
		this.Wait.Done(key, nil)
		this.m.GetAndDel(key)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	data := f.Bytes()
	written := make(chan error, 1)
	go func() {
		_, err := w.write(ctx, data)
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			if ctx.Err() != nil {
				return nil, errors.New("超时")
			}
			return nil, err
		}
	case <-ctx.Done():
		return nil, errors.New("超时")
	}
	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		return nil, errors.New("超时")
	}
}

// getStandardCount 获取标准市场内的证券数量
func (this *Client) getStandardCount(exchange protocol.Exchange) (*protocol.CountResp, error) {
	f := protocol.MCount.Frame(exchange)
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CountResp), nil
}

// getStandardCode 获取市场内指定范围内的所有证券代码,一次固定返回1000只,上证股票有效范围370-1480
// 上证前370只是395/399开头的(中证500/总交易等辅助类),在后面的话是一些100开头的国债
// 600开头的股票是上证A股，属于大盘股，其中6006开头的股票是最早上市的股票， 6016开头的股票为大盘蓝筹股；900开头的股票是上证B股；
// 000开头的股票是深证A股，001、002开头的股票也都属于深证A股， 其中002开头的股票是深证A股中小企业股票；200开头的股票是深证B股；
// 300开头的股票是创业板股票；400开头的股票是三板市场股票。
func (this *Client) getStandardCode(exchange protocol.Exchange, start uint16) (*protocol.CodeResp, error) {
	f := protocol.MCode.Frame(exchange, start)
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CodeResp), nil
}

// getStandardCodeAll 通过多次请求的方式获取全部证券代码
func (this *Client) getStandardCodeAll(exchange protocol.Exchange) (*protocol.CodeResp, error) {
	resp := &protocol.CodeResp{}

	//通达信标准协议(GetCount/GetCode)不支持北交所(market=2),会超时;
	//通过 zhb.zip 中的 tdxbjmore.cfg 获取北交所代码和名称。
	if exchange == protocol.ExchangeBJ {
		files, err := this.GetZHBFiles()
		if err != nil {
			return nil, err
		}
		data, ok := files[protocol.FileTdxBjMore]
		if !ok {
			return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileTdxBjMore)
		}
		resp.List = protocol.ParseTdxBjMore(data)
		resp.Count = uint16(len(resp.List))
		return resp, nil
	}

	size := 1000
	for start := 0; ; start += size {
		if start > 65535 {
			return nil, errors.New("分页偏移超过协议 uint16 上限")
		}
		r, err := this.getStandardCode(exchange, uint16(start))
		if err != nil {
			return nil, err
		}
		if int(resp.Count)+int(r.Count) > 65535 {
			return nil, errors.New("累计记录数超过返回类型 uint16 上限")
		}
		resp.Count += r.Count
		resp.List = append(resp.List, r.List...)
		if int(r.Count) < size {
			break
		}
	}
	return resp, nil
}

// GetStockCodeAll 获取所有股票代码,带前缀例sz000001
func (this *Client) GetStockCodeAll() ([]string, error) {
	ls := []string(nil)
	for _, ex := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ} {
		resp, err := this.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		for _, v := range resp.List {
			if protocol.IsStock(ex.String() + v.Code) {
				ls = append(ls, ex.String()+v.Code)
			}
		}
	}
	return ls, nil
}

// GetETFCodeAll 获取所有ETF代码,带前缀例sz159399
func (this *Client) GetETFCodeAll() ([]string, error) {
	ls := []string(nil)
	for _, ex := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ} {
		resp, err := this.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		for _, v := range resp.List {
			if protocol.IsETF(ex.String() + v.Code) {
				ls = append(ls, ex.String()+v.Code)
			}
		}
	}
	return ls, nil
}

// GetIndexCodeAll 获取所有指数代码,带前缀例sz399001
func (this *Client) GetIndexCodeAll() ([]string, error) {
	ls := []string{"bj899050"}
	for _, ex := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ} {
		resp, err := this.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		for _, v := range resp.List {
			if protocol.IsIndex(ex.String() + v.Code) {
				ls = append(ls, ex.String()+v.Code)
			}
		}
	}
	return ls, nil
}

// getStandardQuotes 按代码元数据校准标准行情价格。
func (this *Client) getStandardQuotes(metadata ICodes, codes ...string) (protocol.QuotesResp, error) {
	for i := range codes {
		//如果是股票代码,则加上前缀
		codes[i] = protocol.AddPrefix(codes[i])
		if !protocol.IsStock(codes[i]) && !protocol.IsIndex(codes[i]) {
			if metadata == nil {
				return nil, errors.New("DefaultCodes未初始化")
			}
			//不是股票/指数代码的话，根据codes的信息加上前缀
			//codes[i] = DefaultCodes.AddExchange(codes[i])
			codes[i] = protocol.AddPrefix(codes[i])
		}
	}

	f, err := protocol.MQuote.Frame(codes...)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	quotes := result.(protocol.QuotesResp)

	{ //todo 临时处理下先,后续优化,感觉有问题
		//判断长度和预期是否一致
		if len(quotes) != len(codes) {
			return nil, fmt.Errorf("预期%d个，实际%d个", len(codes), len(quotes))
		}
		for i, code := range codes {
			// 股票类代码才需要按 Decimal 修正价格;
			// 指数(含板块指数 880xxx)与基金行情原始解码价格即正确, 跳过修正。
			if !protocol.IsStock(code) && !protocol.IsIndex(code) {
				m := metadata.Get(code)
				if m == nil {
					return nil, fmt.Errorf("未查询到代码[%s]相关信息", code)
				}
				for ii, v := range quotes[i].SellLevel {
					quotes[i].SellLevel[ii].Price = m.Price(v.Price)
				}
				for ii, v := range quotes[i].BuyLevel {
					quotes[i].BuyLevel[ii].Price = m.Price(v.Price)
				}
				quotes[i].Kline.Last = m.Price(quotes[i].Kline.Last)
				quotes[i].Kline.Open = m.Price(quotes[i].Kline.Open)
				quotes[i].Kline.High = m.Price(quotes[i].Kline.High)
				quotes[i].Kline.Low = m.Price(quotes[i].Kline.Low)
				quotes[i].Kline.Close = m.Price(quotes[i].Kline.Close)
			}
		}
	}

	return quotes, nil
}

func (this *Client) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	f, err := protocol.MCallAuction.Frame(code)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CallAuctionResp), nil
}

func (this *Client) GetGbbq(code string) (*protocol.GbbqResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MGbbq.Frame(code)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.GbbqResp), nil
}

func (this *Client) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	codes, err := this.GetStockCodeAll()
	if err != nil {
		return nil, err
	}
	gbbqs := map[string][]*protocol.Gbbq{}
	var resp *protocol.GbbqResp
	for _, code := range codes {
		for i := 0; i == 0 || i < DefaultRetry; i++ {
			resp, err = this.GetGbbq(code)
			if err == nil {
				gbbqs[code] = resp.List
				break
			}
		}
		if err != nil {
			return nil, err
		}
	}
	return gbbqs, nil
}

// GetCompanyCategory 获取 F10 公司信息分类目录。
func (this *Client) GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error) {
	r, err := this.SendFrame(protocol.MCompanyCat.Frame(exchange.Uint8(), code))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.CompanyCategory), nil
}

// GetCompanyContent 获取 F10 某分类的文本内容。
func (this *Client) GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error) {
	r, err := this.SendFrame(protocol.MCompanyContent.Frame(exchange.Uint8(), code, filename, start, length))
	if err != nil {
		return "", err
	}
	return r.(string), nil
}

// GetFinanceInfo 获取标的财务/基本面信息（流通股本/总股本/行业/地域/股东户数/财务）。
func (this *Client) GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error) {
	f := protocol.MFinance.Frame(exchange.Uint8(), code)
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.FinanceInfo), nil
}

// GetBlockFileRaw 下载通达信服务器文件（板块/配置）原始字节，分块拉取后拼接。
// 适用于二进制板块文件(block*.dat)与文本配置(tdxhy.cfg 等)。
func (this *Client) GetBlockFileRaw(file string) ([]byte, error) {
	mr, err := this.SendFrame(protocol.MBlock.FrameMeta(file))
	if err != nil {
		return nil, err
	}
	meta, ok := mr.(*protocol.BlockMetaResp)
	if !ok || meta.Size == 0 {
		return nil, fmt.Errorf("板块文件 %s 无数据", file)
	}
	var buf []byte
	start := uint32(0)
	for start < meta.Size {
		n := uint32(0x7530)
		if meta.Size-start < n {
			n = meta.Size - start
		}
		r, err := this.SendFrame(protocol.MBlock.FrameInfo(start, n, file))
		if err != nil {
			return nil, err
		}
		info, ok := r.(*protocol.BlockInfoResp)
		if !ok || len(info.Data) == 0 {
			return nil, fmt.Errorf("文件 %s 下载不完整: 预期 %d 字节，收到 %d: %w", file, meta.Size, start, io.ErrUnexpectedEOF)
		}
		if uint32(len(info.Data)) > n {
			return nil, fmt.Errorf("文件 %s 分块过长: 预期最多 %d 字节，收到 %d", file, n, len(info.Data))
		}
		buf = append(buf, info.Data...)
		start += uint32(len(info.Data))
	}
	return buf, nil
}

// GetBlockData 下载并解析通达信板块文件（如 protocol.BlockFileGN 概念）→ 板块列表。
func (this *Client) GetBlockData(file string) ([]*protocol.Block, error) {
	buf, err := this.GetBlockFileRaw(file)
	if err != nil {
		return nil, err
	}
	return protocol.ParseBlockFile(buf), nil
}

// GetReportFile 下载通达信服务器任意报表/数据文件（report file，指令 0x06B9）原始字节。
// 与 GetBlockFileRaw 共用同一传输帧，区别在于报表文件无 0x02C5 元信息预查文件大小，
// 故按 0x7530 块大小循环递增 offset 拉取，直到返回块短于请求块（末块）或为空时终止。
// 对齐 pytdx GetReportFile / mitdx get_report_file。
func (this *Client) GetReportFile(file string) ([]byte, error) {
	const chunk = uint32(0x7530)
	var buf []byte
	start := uint32(0)
	for {
		r, err := this.SendFrame(protocol.MBlock.FrameInfo(start, chunk, file))
		if err != nil {
			return nil, err
		}
		info, ok := r.(*protocol.BlockInfoResp)
		if !ok || len(info.Data) == 0 {
			break
		}
		buf = append(buf, info.Data...)
		start += uint32(len(info.Data))
		if uint32(len(info.Data)) < chunk {
			break
		}
	}
	return buf, nil
}

// GetZHBFiles 下载板块/配置数据总包 zhb.zip(report file 0x06B9)并解压，返回 文件名→原始字节。
// zhb.zip 内含 tdxzs.cfg(板块指数代码)、tdxbk.cfg(概念板块)、incon.dat(行业分类)等配置文件。
func (this *Client) GetZHBFiles() (map[string][]byte, error) {
	raw, err := this.GetReportFile(protocol.ReportZHB)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s 无数据", protocol.ReportZHB)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("解压 %s 失败: %w", protocol.ReportZHB, err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[f.Name] = b
	}
	return out, nil
}

// GetTdxZs 下载并解析板块指数配置 tdxzs.cfg(来自 zhb.zip) → 板块名↔指数代码(id) 列表。
func (this *Client) GetTdxZs() ([]*protocol.TdxZs, error) {
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[protocol.FileTdxZs]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileTdxZs)
	}
	return protocol.ParseTdxZs(data), nil
}

// GetTdxBk 下载并解析 tdxbk.cfg(来自 zhb.zip) → 概念板块简称↔全称。
func (this *Client) GetTdxBk() ([]*protocol.TdxBk, error) {
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[protocol.FileTdxBk]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileTdxBk)
	}
	return protocol.ParseTdxBk(data), nil
}

// GetBlockDataWithIndex 下载板块文件(block_*.dat)并按名称回填板块指数代码(id)。
// block 文件本身无 id，关联链: 板块名 →(tdxzs.cfg)→ id；直接未命中再经
// 简称 →(tdxbk.cfg)→ 全称 →(tdxzs.cfg)→ id 二次匹配。三个文件均来自 zhb.zip(仅下载一次)。
func (this *Client) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	blocks, err := this.GetBlockData(file)
	if err != nil {
		return nil, err
	}
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	zs := protocol.ParseTdxZs(files[protocol.FileTdxZs])
	bk := protocol.ParseTdxBk(files[protocol.FileTdxBk])
	protocol.FillBlockIndexAlias(blocks, zs, bk)
	return blocks, nil
}

// GetTdxStat 下载并解析 tdxstat.cfg(来自 zhb.zip) → 全市场个股综合统计指标。
func (this *Client) GetTdxStat() ([]*protocol.TdxStat, error) {
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[protocol.FileTdxStat]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileTdxStat)
	}
	return protocol.ParseTdxStat(data), nil
}

// GetTdxStat2 下载并解析 tdxstat2.cfg(来自 zhb.zip) → 全市场个股资金流向 + 板块归属。
// 其 BlockIndex 字段提供 股→板块指数代码(id) 的反向映射(见 protocol.StockBlockIndex)。
func (this *Client) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[protocol.FileTdxStat2]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileTdxStat2)
	}
	return protocol.ParseTdxStat2(data), nil
}

// GetXgsg 下载并解析 xgsg.cfg(来自 zhb.zip) → 新股申购列表。
func (this *Client) GetXgsg() ([]*protocol.TdxXgsg, error) {
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[protocol.FileXgsg]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileXgsg)
	}
	return protocol.ParseXgsg(data), nil
}

// GetSpBlock 下载并解析 spblock.dat(来自 zhb.zip) → 专业板块成分列表。
//
// spblock.dat 承载 block_zs.dat 无法容纳的大型指数成分, 如
// 中证2000/中证1000/中证500/中证A500/国证2000/深证成指 等(block_*.dat 单板块成分上限 400)。
// 用 protocol.SpBlockByName 按名取单个板块。注意: 沪深300 不在此文件, 在 block_zs.dat(见 GetBlockData)。
func (this *Client) GetSpBlock() ([]*protocol.SpBlock, error) {
	files, err := this.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[protocol.FileSpBlock]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, protocol.FileSpBlock)
	}
	return protocol.ParseSpBlock(data), nil
}

// GetTdxHy 下载并解析 tdxhy.cfg → 每只股票的通达信/申万行业归属。
func (this *Client) GetTdxHy() ([]*protocol.TdxHy, error) {
	buf, err := this.GetBlockFileRaw(protocol.FileTdxHy)
	if err != nil {
		return nil, err
	}
	return protocol.ParseTdxHy(buf), nil
}

// getStandardMinute 获取标准分时数据,todo 解析好像不对,先用历史数据
func (this *Client) getStandardMinute(code string) (*protocol.MinuteResp, error) {
	// 实时分时解析存疑，移植后暂统一走历史分时（去除原 unreachable 实现以过 vet）。
	return this.getStandardHistoryMinute(time.Now().Format("20060102"), code)
}

// getStandardHistoryMinute 获取标准历史分时数据
func (this *Client) getStandardHistoryMinute(date, code string) (*protocol.MinuteResp, error) {
	f, err := protocol.MHistoryMinute.Frame(date, code)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.MinuteResp), nil
}

// getStandardTrade 获取标准分时交易详情,服务器最多返回1800条,count-start<=1800
func (this *Client) getStandardTrade(code string, start, count uint16) (*protocol.TradeResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MTrade.Frame(code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f, protocol.TradeCache{
		Date: time.Now().Format("20060102"),
		Code: code,
	})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.TradeResp), nil
}

func (this *Client) getStandardHistoryTrade(date, code string, start, count uint16) (*protocol.TradeResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MHistoryTrade.Frame(date, code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f, protocol.TradeCache{
		Date: date,
		Code: code,
	})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.TradeResp), nil
}

func (this *Client) getStandardKline(Type uint8, code string, start, count uint16, kind string) (*protocol.KlineResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MKline.Frame(Type, code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f, protocol.KlineCache{Type: Type, Kind: kind, Code: code})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.KlineResp), nil
}
