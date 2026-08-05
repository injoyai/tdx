package tdx

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync/atomic"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/module/common"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/lib/bse"
	"github.com/injoyai/tdx/protocol"
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

	cli = &Client{
		Wait: wait.New(time.Second * 2),
		m:    maps.NewSafe(),
	}

	cli.Client, err = client.Dial(dial, func(c *client.Client) {
		c.Logger.Debug(true)                           //关闭日志打印
		c.Logger.SetLevel(LevelInfo)                   //设置日志级别
		c.Logger.WithHEX()                             //以HEX显示
		c.SetOption(op...)                             //自定义选项
		c.Event.OnReadFrom = protocol.ReadFrom         //分包
		c.Event.OnDealMessage = cli.handlerDealMessage //解析数据并处理
		c.Event.OnConnected = func(c *client.Client) error {
			//无数据超时时间是60秒,30秒发送一个心跳包
			c.GoTimerWriter(30*time.Second, func(w ios.MoreWriter) error {
				bs := protocol.MHeart.Frame().Bytes()
				_, err := w.Write(bs)
				return err
			})
			f := protocol.MConnect.Frame()
			if _, err = c.Write(f.Bytes()); err != nil {
				c.Close()
			}
			return nil
		}
	})
	if err != nil {
		return nil, err
	}

	go cli.Client.Run()

	return cli, err
}

type Client struct {
	*client.Client              //客户端实例
	Wait           *wait.Entity //异步回调,设置超时时间,超时则返回错误
	m              *maps.Safe   //有部分解析需要用到代码,返回数据获取不到,固请求的时候缓存下
	msgID          uint32       //消息id,使用SendFrame自动累加
}

// handlerDealMessage 处理服务器响应的数据
func (this *Client) handlerDealMessage(c *client.Client, msg ios.Acker) {

	defer func() {
		if e := recover(); e != nil {
			logs.Err(e)
			debug.PrintStack()
		}
	}()

	f, err := protocol.Decode(msg.Payload())
	if err != nil {
		logs.Err(err)
		return
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
		resp = protocol.MQuote.Decode(f.Data)

	case protocol.TypeMinute:
		resp, err = protocol.MMinute.Decode(f.Data)

	case protocol.TypeHistoryMinute:
		resp, err = protocol.MHistoryMinute.Decode(f.Data)

	case protocol.TypeCallAuction:
		resp, err = protocol.MCallAuction.Decode(f.Data)

	case protocol.TypeMinuteTrade:
		resp, err = protocol.MTrade.Decode(f.Data, val.(protocol.TradeCache))

	case protocol.TypeHistoryMinuteTrade:
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
		return
	}

	this.Wait.Done(conv.String(f.MsgID), resp)

}

// SetTimeout 设置超时时间
func (this *Client) SetTimeout(t time.Duration) {
	this.Wait.SetTimeout(t)
}

// SendFrame 发送数据,并等待响应
func (this *Client) SendFrame(f *protocol.Frame, cache ...any) (any, error) {
	f.MsgID = atomic.AddUint32(&this.msgID, 1)
	if len(cache) > 0 {
		this.m.Set(conv.String(f.MsgID), cache[0])
	}
	if _, err := this.Client.Write(f.Bytes()); err != nil {
		return nil, err
	}
	return this.Wait.Wait(conv.String(f.MsgID))
}

// GetCount 获取市场内的股票数量
func (this *Client) GetCount(exchange protocol.Exchange) (*protocol.CountResp, error) {
	f := protocol.MCount.Frame(exchange)
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CountResp), nil
}

// GetCode 获取市场内指定范围内的所有证券代码,一次固定返回1000只,上证股票有效范围370-1480
// 上证前370只是395/399开头的(中证500/总交易等辅助类),在后面的话是一些100开头的国债
// 600开头的股票是上证A股，属于大盘股，其中6006开头的股票是最早上市的股票， 6016开头的股票为大盘蓝筹股；900开头的股票是上证B股；
// 000开头的股票是深证A股，001、002开头的股票也都属于深证A股， 其中002开头的股票是深证A股中小企业股票；200开头的股票是深证B股；
// 300开头的股票是创业板股票；400开头的股票是三板市场股票。
func (this *Client) GetCode(exchange protocol.Exchange, start uint16) (*protocol.CodeResp, error) {
	f := protocol.MCode.Frame(exchange, start)
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CodeResp), nil
}

// GetCodeAll 通过多次请求的方式获取全部证券代码
func (this *Client) GetCodeAll(exchange protocol.Exchange) (*protocol.CodeResp, error) {
	resp := &protocol.CodeResp{}

	//通达信没有北交所代码列表,通过爬虫的方式从北交所官网获取,放在这里是为了方便业务逻辑
	//不放在extend包时防止循环引用
	//todo 这是临时方案,等通达信有北交所代码列表时再改
	if exchange == protocol.ExchangeBJ {
		codes, err := bse.GetCodes()
		if err != nil {
			return nil, err
		}
		resp.Count = uint16(len(codes))
		for _, v := range codes {
			resp.List = append(resp.List, &protocol.Code{
				Code:      v.Code,
				Name:      v.Name,
				LastPrice: v.Last,
			})
		}
		return resp, nil
	}

	size := uint16(1000)
	for start := uint16(0); ; start += size {
		r, err := this.GetCode(exchange, start)
		if err != nil {
			return nil, err
		}
		resp.Count += r.Count
		resp.List = append(resp.List, r.List...)
		if r.Count < size {
			break
		}
	}
	return resp, nil
}

// GetStockCodeAll 获取所有股票代码,带前缀例sz000001
func (this *Client) GetStockCodeAll() ([]string, error) {
	ls := []string(nil)
	for _, ex := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ} {
		resp, err := this.GetCodeAll(ex)
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
		resp, err := this.GetCodeAll(ex)
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
		resp, err := this.GetCodeAll(ex)
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

// GetQuote 获取盘口五档报价
func (this *Client) GetQuote(codes ...string) (protocol.QuotesResp, error) {
	for i := range codes {
		//如果是股票代码,则加上前缀
		codes[i] = protocol.AddPrefix(codes[i])
		if !protocol.IsStock(codes[i]) {
			if DefaultCodes == nil {
				return nil, errors.New("DefaultCodes未初始化")
			}
			//不是股票代码的话，根据codes的信息加上前缀
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
			if !protocol.IsStock(code) {
				m := DefaultCodes.Get(code)
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
			break
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

// GetMinute 获取分时数据,todo 解析好像不对,先用历史数据
func (this *Client) GetMinute(code string) (*protocol.MinuteResp, error) {
	// 实时分时解析存疑，移植后暂统一走历史分时（去除原 unreachable 实现以过 vet）。
	return this.GetHistoryMinute(time.Now().Format("20060102"), code)
}

// GetHistoryMinute 获取历史分时数据
func (this *Client) GetHistoryMinute(date, code string) (*protocol.MinuteResp, error) {
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

func (this *Client) GetTrade(code string, start, count uint16) (*protocol.TradeResp, error) {
	return this.GetMinuteTrade(code, start, count)
}

// GetMinuteTrade 获取分时交易详情,服务器最多返回1800条,count-start<=1800
func (this *Client) GetMinuteTrade(code string, start, count uint16) (*protocol.TradeResp, error) {
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

func (this *Client) GetTradeAll(code string) (*protocol.TradeResp, error) {
	return this.GetMinuteTradeAll(code)
}

// GetMinuteTradeAll 获取分时全部交易详情,todo 只做参考 因为交易实时在进行,然后又是分页读取的,所以会出现读取间隔内产生的交易会丢失
func (this *Client) GetMinuteTradeAll(code string) (*protocol.TradeResp, error) {
	resp := &protocol.TradeResp{}
	size := uint16(1800)
	for start := uint16(0); ; start += size {
		r, err := this.GetMinuteTrade(code, start, size)
		if err != nil {
			return nil, err
		}
		resp.Count += r.Count
		resp.List = append(r.List, resp.List...)

		if r.Count < size {
			break
		}
	}
	return resp, nil
}

func (this *Client) GetHistoryTrade(date, code string, start, count uint16) (*protocol.TradeResp, error) {
	return this.GetHistoryMinuteTrade(date, code, start, count)
}

// GetHistoryMinuteTrade 获取历史分时交易
// 只能获取昨天及之前的数据,服务器最多返回2000条,count-start<=2000,如果日期输入错误,则返回0
// 历史数据只能查到20000609
func (this *Client) GetHistoryMinuteTrade(date, code string, start, count uint16) (*protocol.TradeResp, error) {
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

// GetHistoryTradeFull 获取上市至今的分时成交
func (this *Client) GetHistoryTradeFull(code string, w *Workday) (protocol.Trades, error) {
	return this.GetHistoryTradeBefore(code, w, time.Now())
}

// GetHistoryTradeBefore 获取上市至今的分时成交
func (this *Client) GetHistoryTradeBefore(code string, w *Workday, before time.Time) (protocol.Trades, error) {
	ls := protocol.Trades(nil)
	resp, err := this.GetKlineMonthAll(code)
	if err != nil {
		return nil, err
	}
	if len(resp.List) == 0 {
		return nil, nil
	}
	start := time.Date(resp.List[0].Time.Year(), resp.List[0].Time.Month(), 1, 0, 0, 0, 0, resp.List[0].Time.Location())
	var res *protocol.TradeResp
	w.Range(start, before, func(t time.Time) bool {
		for i := 0; i < 3; i++ {
			res, err = this.GetHistoryTradeDay(t.Format("20060102"), code)
			if err == nil {
				break
			}
		}
		if err != nil {
			return false
		}
		ls = append(ls, res.List...)
		return true
	})
	return ls, err
}

// GetHistoryTradeDay 获取历史某天分时全部交易,通过多次请求来拼接,只能获取昨天及之前的数据
func (this *Client) GetHistoryTradeDay(date, code string) (*protocol.TradeResp, error) {
	return this.GetHistoryMinuteTradeDay(date, code)
}

// GetHistoryMinuteTradeDay 获取历史某天分时全部交易,通过多次请求来拼接,只能获取昨天及之前的数据
// 历史数据只能查到20000609
func (this *Client) GetHistoryMinuteTradeDay(date, code string) (*protocol.TradeResp, error) {
	resp := &protocol.TradeResp{}
	size := uint16(2000)
	for start := uint16(0); ; start += size {
		r, err := this.GetHistoryMinuteTrade(date, code, start, size)
		if err != nil {
			return nil, err
		}
		resp.Count += r.Count
		resp.List = append(r.List, resp.List...)
		if r.Count < size {
			break
		}
	}
	return resp, nil
}

/*



 */

// GetIndex 获取指数,接口是和k线一样的,但是解析不知道怎么区分(解析方式不一致),所以加一个方法
func (this *Client) GetIndex(Type uint8, code string, start, count uint16) (*protocol.KlineResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MKline.Frame(Type, code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f, protocol.KlineCache{Type: Type, Kind: protocol.KindIndex, Code: code})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.KlineResp), nil
}

// GetIndexUntil 获取指数k线数据，通过多次请求来拼接,直到满足func返回true
func (this *Client) GetIndexUntil(Type uint8, code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	resp := &protocol.KlineResp{}
	size := uint16(800)
	var last *protocol.Kline
	for start := uint16(0); ; start += size {
		r, err := this.GetIndex(Type, code, start, size)
		if err != nil {
			return nil, err
		}
		if last != nil && len(r.List) > 0 {
			last.Last = r.List[len(r.List)-1].Close
		}
		if len(r.List) > 0 {
			last = r.List[0]
		}
		for i := len(r.List) - 1; i >= 0; i-- {
			if f(r.List[i]) {
				resp.Count += r.Count - uint16(i)
				resp.List = append(r.List[i:], resp.List...)
				return resp, nil
			}
		}
		resp.Count += r.Count
		resp.List = append(r.List, resp.List...)
		if r.Count < size {
			break
		}
	}
	return resp, nil
}

// GetIndexAll 获取全部k线数据
func (this *Client) GetIndexAll(Type uint8, code string) (*protocol.KlineResp, error) {
	return this.GetIndexUntil(Type, code, func(k *protocol.Kline) bool { return false })
}

func (this *Client) GetIndexMinute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetIndex(protocol.TypeKlineMinute, code, start, count)
}

func (this *Client) GetIndex5Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetIndex(protocol.TypeKline5Minute, code, start, count)
}

func (this *Client) GetIndex15Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetIndex(protocol.TypeKline15Minute, code, start, count)
}

func (this *Client) GetIndex30Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetIndex(protocol.TypeKline30Minute, code, start, count)
}

func (this *Client) GetIndex60Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetIndex(protocol.TypeKline60Minute, code, start, count)
}

func (this *Client) GetIndexDay(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetIndex(protocol.TypeKlineDay, code, start, count)
}

func (this *Client) GetIndexDayUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetIndexUntil(protocol.TypeKlineDay, code, f)
}

func (this *Client) GetIndexDayAll(code string) (*protocol.KlineResp, error) {
	return this.GetIndexAll(protocol.TypeKlineDay, code)
}

func (this *Client) GetIndexWeekAll(code string) (*protocol.KlineResp, error) {
	return this.GetIndexAll(protocol.TypeKlineWeek, code)
}

func (this *Client) GetIndexMonthAll(code string) (*protocol.KlineResp, error) {
	return this.GetIndexAll(protocol.TypeKlineMonth, code)
}

func (this *Client) GetIndexQuarterAll(code string) (*protocol.KlineResp, error) {
	return this.GetIndexAll(protocol.TypeKlineQuarter, code)
}

func (this *Client) GetIndexYearAll(code string) (*protocol.KlineResp, error) {
	return this.GetIndexAll(protocol.TypeKlineYear, code)
}

/*


 */

// GetKline 获取k线数据,推荐收盘之后获取,否则会获取到当天的数据
func (this *Client) GetKline(Type uint8, code string, start, count uint16) (*protocol.KlineResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MKline.Frame(Type, code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f, protocol.KlineCache{Type: Type, Kind: protocol.KindStock, Code: code})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.KlineResp), nil
}

// GetKlineUntil 获取k线数据，通过多次请求来拼接,直到满足func返回true
func (this *Client) GetKlineUntil(Type uint8, code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	resp := &protocol.KlineResp{}
	size := uint16(800)
	var last *protocol.Kline
	for start := uint16(0); ; start += size {
		r, err := this.GetKline(Type, code, start, size)
		if err != nil {
			return nil, err
		}
		if last != nil && len(r.List) > 0 {
			last.Last = r.List[len(r.List)-1].Close
		}
		if len(r.List) > 0 {
			last = r.List[0]
		}
		for i := len(r.List) - 1; i >= 0; i-- {
			if f(r.List[i]) {
				resp.Count += r.Count - uint16(i)
				resp.List = append(r.List[i:], resp.List...)
				return resp, nil
			}
		}
		resp.Count += r.Count
		resp.List = append(r.List, resp.List...)
		if r.Count < size {
			break
		}
	}
	return resp, nil
}

// GetKlineAll 获取全部k线数据
func (this *Client) GetKlineAll(Type uint8, code string) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(Type, code, func(k *protocol.Kline) bool { return false })
}

// GetKlineMinute 获取一分钟k线数据,每次最多800条,最多只能获取24000条数据
func (this *Client) GetKlineMinute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKlineMinute, code, start, count)
}

// GetKlineMinuteAll 获取一分钟k线全部数据,最多只能获取24000条数据
func (this *Client) GetKlineMinuteAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKlineMinute, code)
}

func (this *Client) GetKlineMinuteUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKlineMinute, code, f)
}

func (this *Client) GetKlineMinute241Until(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	resp, err := this.GetKlineMinuteUntil(code, f)
	if err != nil {
		return nil, err
	}
	if len(resp.List) == 0 {
		return resp, nil
	}
	ks := protocol.Klines{}
	for _, v := range resp.List {
		if v.Time.Format(time.TimeOnly) == "09:31:00" {
			var tr *protocol.TradeResp
			if v.Time.Format(time.DateOnly) == time.Now().Format(time.DateOnly) {
				tr, err = this.GetTradeAll(code)
			} else {
				tr, err = this.GetHistoryTradeDay(v.Time.Format("20060102"), code)
			}
			if err != nil {
				return nil, err
			}
			_925 := new(protocol.Trade)
			if len(tr.List) > 0 && tr.List[0].Time.Format(time.TimeOnly) < "09:30:00" {
				_925 = tr.List[0]
			}
			ks = append(ks, &protocol.Kline{
				Last:   v.Last,
				Open:   _925.Price,
				High:   _925.Price,
				Low:    _925.Price,
				Close:  _925.Price,
				Order:  _925.Number,
				Volume: int64(_925.Volume),
				Amount: _925.Amount(),
				Time:   time.Date(v.Time.Year(), v.Time.Month(), v.Time.Day(), 9, 30, 0, 0, v.Time.Location()),
			})
			v.Last = _925.Price
			v.Volume -= int64(_925.Volume)
			if v.Volume < 0 {
				v.Volume = 0
			}
			v.Amount -= _925.Amount()
			if v.Amount < 0 {
				v.Amount = 0
			}
			v.Order -= _925.Number
			if v.Order < 0 {
				v.Order = 0
			}
		}
		ks = append(ks, v)
	}
	resp.List = ks
	resp.Count = uint16(len(ks))
	return resp, nil
}

// GetKline5Minute 获取五分钟k线数据
func (this *Client) GetKline5Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKline5Minute, code, start, count)
}

// GetKline5MinuteAll 获取5分钟k线全部数据
func (this *Client) GetKline5MinuteAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKline5Minute, code)
}

func (this *Client) GetKline5MinuteUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKline5Minute, code, f)
}

// GetKline15Minute 获取十五分钟k线数据
func (this *Client) GetKline15Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKline15Minute, code, start, count)
}

// GetKline15MinuteAll 获取十五分钟k线全部数据
func (this *Client) GetKline15MinuteAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKline15Minute, code)
}

func (this *Client) GetKline15MinuteUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKline15Minute, code, f)
}

// GetKline30Minute 获取三十分钟k线数据
func (this *Client) GetKline30Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKline30Minute, code, start, count)
}

// GetKline30MinuteAll 获取三十分钟k线全部数据
func (this *Client) GetKline30MinuteAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKline30Minute, code)
}

func (this *Client) GetKline30MinuteUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKline30Minute, code, f)
}

// GetKline60Minute 获取60分钟k线数据
func (this *Client) GetKline60Minute(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKline60Minute, code, start, count)
}

// GetKlineHour 获取小时k线数据
func (this *Client) GetKlineHour(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKline60Minute, code, start, count)
}

// GetKline60MinuteAll 获取60分钟k线全部数据
func (this *Client) GetKline60MinuteAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKline60Minute, code)
}

// GetKlineHourAll 获取小时k线全部数据
func (this *Client) GetKlineHourAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKline60Minute, code)
}

func (this *Client) GetKline60MinuteUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKline60Minute, code, f)
}

func (this *Client) GetKlineHourUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKline60Minute, code, f)
}

// GetKlineDay 获取日k线数据
func (this *Client) GetKlineDay(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKlineDay, code, start, count)
}

// GetKlineDayAll 获取日k线全部数据
func (this *Client) GetKlineDayAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKlineDay, code)
}

func (this *Client) GetKlineDayUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKlineDay, code, f)
}

// GetKlineWeek 获取周k线数据
func (this *Client) GetKlineWeek(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKlineWeek, code, start, count)
}

// GetKlineWeekAll 获取周k线全部数据
func (this *Client) GetKlineWeekAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKlineWeek, code)
}

func (this *Client) GetKlineWeekUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKlineWeek, code, f)
}

// GetKlineMonth 获取月k线数据
func (this *Client) GetKlineMonth(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKlineMonth, code, start, count)
}

// GetKlineMonthAll 获取月k线全部数据
func (this *Client) GetKlineMonthAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKlineMonth, code)
}

func (this *Client) GetKlineMonthUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKlineMonth, code, f)
}

// GetKlineQuarter 获取季k线数据
func (this *Client) GetKlineQuarter(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKlineQuarter, code, start, count)
}

// GetKlineQuarterAll 获取季k线全部数据
func (this *Client) GetKlineQuarterAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKlineQuarter, code)
}

func (this *Client) GetKlineQuarterUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKlineQuarter, code, f)
}

// GetKlineYear 获取年k线数据
func (this *Client) GetKlineYear(code string, start, count uint16) (*protocol.KlineResp, error) {
	return this.GetKline(protocol.TypeKlineYear, code, start, count)
}

// GetKlineYearAll 获取年k线数据
func (this *Client) GetKlineYearAll(code string) (*protocol.KlineResp, error) {
	return this.GetKlineAll(protocol.TypeKlineYear, code)
}

func (this *Client) GetKlineYearUntil(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error) {
	return this.GetKlineUntil(protocol.TypeKlineYear, code, f)
}
