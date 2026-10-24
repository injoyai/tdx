package tdx

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/internal/codecache"
	"github.com/injoyai/tdx/v2/protocol"
)

const stPort = "7709"

// StConn 是绑定标准行情协议的业务连接，实现 Client，复用内部 conn 的收发、超时和重连管理。
// Dial* 直接返回该类型，不需要再包装通用连接。
type StConn struct {
	*conn
	*queries
	quoteMu    sync.Mutex
	quoteCodes *codecache.Base
}

// GetKline 请求标准 K 线，保留整数价格及指数特有字段。
func (c *StConn) GetKline(category uint8, code string, start, count uint16) (*KlineResp, error) {
	out, info, err := klineRequest(code, category, count)
	if err != nil {
		return nil, err
	}
	if _, _, err := protocolInstrument(code, false); err != nil {
		return nil, err
	}
	scale, err := klineVolumeScale(out.Instrument, out.Category, info)
	if err != nil {
		return nil, err
	}
	r, err := c.getStandardKline(out.Category, standardCode(out.Instrument), start, count, info.Kind)
	if err != nil {
		return nil, err
	}
	for _, k := range r.List {
		v := standardKline(k)
		v.Volume, err = scaleVolume(k.Volume, scale)
		if err != nil {
			return nil, err
		}
		v.Time = protocol.KlineTime(k.Time, out.Category)
		out.List = append(out.List, v)
	}
	if len(out.List) > int(count) {
		return nil, errors.New("K线响应数量超过请求数量")
	}
	out.Count = len(out.List)
	return out, nil
}

// GetQuote 获取标准五档报价，保持输入顺序及重复代码。
func (c *StConn) GetQuote(codes ...string) ([]Quote, error) {
	instruments := make([]protocol.Instrument, len(codes))
	standard := make([]string, len(codes))
	for j, code := range codes {
		i, _, err := protocolInstrument(code, false)
		if err != nil {
			return nil, err
		}
		instruments[j], standard[j] = i, standardCode(i)
	}
	if len(standard) > (65535-12)/7 {
		return nil, errors.New("标准报价请求超过协议 uint16 帧长上限")
	}
	out := make([]Quote, len(codes))
	if len(codes) == 0 {
		return out, nil
	}
	quotes, err := c.standardQuotes(standard)
	if err != nil {
		return nil, err
	}
	for j, q := range quotes {
		i := instruments[j]
		if q.Exchange != i.Exchange || q.Code != i.Code {
			return nil, fmt.Errorf("标准行情响应品种与请求不一致: %s", i)
		}
		k := q.Kline
		u := Quote{Instrument: i, PreClose: k.Last.Float64(), Open: k.Open.Float64(), High: k.High.Float64(), Low: k.Low.Float64(), Price: k.Close.Float64(), Volume: k.Volume, Standard: q}
		for n := 0; n < 5; n++ {
			u.Bid[n], u.BidVolume[n] = q.BuyLevel[n].Price.Float64(), int64(q.BuyLevel[n].Number)
			u.Ask[n], u.AskVolume[n] = q.SellLevel[n].Price.Float64(), int64(q.SellLevel[n].Number)
		}
		scale, scaleErr := volumeScale(i)
		if scaleErr != nil {
			// 零量不需要倍率；保留无成交品种的价格和精度元数据查询。
			if u.Volume != 0 || u.BidVolume != [5]int64{} || u.AskVolume != [5]int64{} {
				return nil, scaleErr
			}
			scale = 1
		}
		u.Volume, err = scaleVolume(u.Volume, scale)
		if err != nil {
			return nil, err
		}
		for n := 0; n < 5; n++ {
			u.BidVolume[n], err = scaleVolume(u.BidVolume[n], scale)
			if err != nil {
				return nil, err
			}
			u.AskVolume[n], err = scaleVolume(u.AskVolume[n], scale)
			if err != nil {
				return nil, err
			}
		}
		out[j] = u
	}
	return out, nil
}

// standardQuotes 在需要价格精度校准时取元数据，避免额外数据库/连接。
func (c *StConn) standardQuotes(codes []string) (protocol.QuotesResp, error) {
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
		r, err := c.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		var models []*codecache.CodeModel
		for _, m := range c.quoteCodes.Iter() {
			if m.Exchange != ex.String() {
				models = append(models, m)
			}
		}
		for _, m := range r.List {
			models = append(models, &codecache.CodeModel{Exchange: ex.String(), Code: m.Code, Name: m.Name, Decimal: m.Decimal, Multiple: m.Multiple, LastPrice: m.LastPrice})
		}
		c.quoteCodes.Update(models)
		loaded[ex] = true
	}
	return c.getStandardQuotes(c.quoteCodes, codes...)
}

// GetCode 获取标准市场代码的一页；北交所使用完整报表列表后分页。
func (c *StConn) GetCode(ex protocol.Exchange, start uint32, count uint16) (*CodeResp, error) {
	if err := codePageBounds(ex, start, count); err != nil {
		return nil, err
	}
	if err := protocolExchange(ex, false); err != nil {
		return nil, err
	}
	if ex == protocol.ExchangeBJ {
		out, err := c.GetCodeAll(ex)
		if err != nil {
			return nil, err
		}
		return codePage(out, start, count), nil
	}
	r, err := c.getStandardCode(ex, uint16(start))
	if err != nil {
		return nil, err
	}
	return codePage(standardCodes(ex, r), 0, count), nil
}

// GetCodeAll 获取标准市场完整代码列表，北交所沿用报表来源。
func (c *StConn) GetCodeAll(ex protocol.Exchange) (*CodeResp, error) {
	if err := protocolExchange(ex, false); err != nil {
		return nil, err
	}
	r, err := c.getStandardCodeAll(ex)
	if err != nil {
		return nil, err
	}
	return standardCodes(ex, r), nil
}

// GetCount 返回标准市场品种数量；北交所按报表列表统计。
func (c *StConn) GetCount(ex protocol.Exchange) (int, error) {
	if err := protocolExchange(ex, false); err != nil {
		return 0, err
	}
	if ex == protocol.ExchangeBJ {
		r, err := c.GetCodeAll(ex)
		if err != nil {
			return 0, err
		}
		return r.Count, nil
	}
	r, err := c.getStandardCount(ex)
	if err != nil {
		return 0, err
	}
	return int(r.Count), nil
}

func (c *StConn) minutePage(date, code string) (*MinuteResp, error) {
	i, _, err := protocolInstrument(code, false)
	if err != nil {
		return nil, err
	}
	scale, err := volumeScale(i)
	if err != nil {
		return nil, err
	}
	out := &MinuteResp{Instrument: i, Date: date, TimeZone: protocol.Location().String()}
	var r *protocol.MinuteResp
	if date == "" {
		r, err = c.getStandardMinute(standardCode(i))
	} else {
		r, err = c.getStandardHistoryMinute(date, standardCode(i))
	}
	if err != nil {
		return nil, err
	}
	for j := range r.List {
		k := &r.List[j]
		volume, err := scaleVolume(int64(k.Number), scale)
		if err != nil {
			return nil, err
		}
		out.List = append(out.List, Minute{Time: k.Time, Price: k.Price.Float64(), Volume: volume, Standard: k})
	}

	out.Count = len(out.List)
	return out, nil
}

func (c *StConn) tradePage(date, code string, start, count uint16) (*TradeResp, error) {
	i, _, err := protocolInstrument(code, false)
	if err != nil {
		return nil, err
	}
	scale, err := volumeScale(i)
	if err != nil {
		return nil, err
	}
	out := &TradeResp{Instrument: i, Date: date, TimeZone: protocol.Location().String()}
	var r *protocol.TradeResp
	if date == "" {
		r, err = c.getStandardTrade(standardCode(i), start, count)
	} else {
		r, err = c.getStandardHistoryTrade(date, standardCode(i), start, count)
	}
	if err != nil {
		return nil, err
	}
	for _, k := range r.List {
		volume, err := scaleVolume(int64(k.Volume), scale)
		if err != nil {
			return nil, err
		}
		price := k.Price.Float64()
		direction := 0
		if k.IsBuy() {
			direction = 1
		} else if k.IsSell() {
			direction = -1
		}
		out.List = append(out.List, Trade{Date: date, Time: k.Time.Format("15:04:05"), Price: &price, Volume: volume, Direction: direction, Standard: k})
	}

	out.Count = len(out.List)
	return out, nil
}

// GetKlineRange 通过标准一分钟分页筛选日期区间，包含两端日期。
func (c *StConn) GetKlineRange(code, startDate, endDate string) (*KlineResp, error) {
	_, _, fromDay, toDay, err := klineRangeDates(startDate, endDate)
	if err != nil {
		return nil, err
	}
	if _, _, err := protocolInstrument(code, false); err != nil {
		return nil, err
	}
	out, err := c.GetKlineUntil(protocol.TypeKlineMinute, code, func(k Kline) bool { return strings.SplitN(k.Time, " ", 2)[0] < fromDay })
	if err != nil {
		return nil, err
	}
	return filterKlineRange(out, fromDay, toDay), nil
}

// GetMarkets 返回内置标准市场，不建立扩展连接。
func (c *StConn) GetMarkets() ([]Exchange, error) {
	select {
	case <-c.Done():
		return nil, net.ErrClosed
	default:
	}
	return standardMarkets(), nil
}

// getStandardCount 获取标准市场内的证券数量
func (c *StConn) getStandardCount(exchange protocol.Exchange) (*protocol.CountResp, error) {
	f := protocol.MCount.Frame(exchange)
	result, err := c.SendFrame(f)
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
func (c *StConn) getStandardCode(exchange protocol.Exchange, start uint16) (*protocol.CodeResp, error) {
	f := protocol.MCode.Frame(exchange, start)
	result, err := c.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CodeResp), nil
}

// getStandardCodeAll 通过多次请求的方式获取全部证券代码
func (c *StConn) getStandardCodeAll(exchange protocol.Exchange) (*protocol.CodeResp, error) {
	resp := &protocol.CodeResp{}

	//通达信标准协议(GetCount/GetCode)不支持北交所(market=2),会超时;
	//通过 zhb.zip 中的 tdxbjmore.cfg 获取北交所代码和名称。
	if exchange == protocol.ExchangeBJ {
		files, err := c.GetZHBFiles()
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
		r, err := c.getStandardCode(exchange, uint16(start))
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
func (c *StConn) GetStockCodeAll() ([]string, error) {
	return c.codeList(nil, protocol.IsStock, protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ)
}

// GetETFCodeAll 获取所有ETF代码,带前缀例sz159399
func (c *StConn) GetETFCodeAll() ([]string, error) {
	return c.codeList(nil, protocol.IsETF, protocol.ExchangeSH, protocol.ExchangeSZ)
}

// GetIndexCodeAll 获取所有指数代码,带前缀例sz399001
func (c *StConn) GetIndexCodeAll() ([]string, error) {
	return c.codeList([]string{"bj899050"}, protocol.IsIndex, protocol.ExchangeSH, protocol.ExchangeSZ)
}

func (c *StConn) codeList(ls []string, match func(string) bool, exchanges ...protocol.Exchange) ([]string, error) {
	for _, ex := range exchanges {
		resp, err := c.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		for _, v := range resp.List {
			if code := ex.String() + v.Code; match(code) {
				ls = append(ls, code)
			}
		}
	}
	return ls, nil
}

// getStandardQuotes 按代码元数据校准标准行情价格。
func (c *StConn) getStandardQuotes(metadata *codecache.Base, codes ...string) (protocol.QuotesResp, error) {
	for i := range codes {
		//如果是股票代码,则加上前缀
		codes[i] = protocol.AddPrefix(codes[i])
		if !protocol.IsStock(codes[i]) && !protocol.IsIndex(codes[i]) {
			if metadata == nil {
				return nil, errors.New("代码元数据未初始化")
			}
			codes[i] = protocol.AddPrefix(codes[i])
		}
	}

	f, err := protocol.MQuote.Frame(codes...)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f)
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

func (c *StConn) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	i, err := standardInstrument(code)
	if err != nil {
		return nil, err
	}
	code = standardCode(i)
	f, err := protocol.MCallAuction.Frame(code)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.CallAuctionResp), nil
}

func (c *StConn) GetGbbq(code string) (*protocol.GbbqResp, error) {
	i, err := standardInstrument(code)
	if err != nil {
		return nil, err
	}
	code = standardCode(i)
	f, err := protocol.MGbbq.Frame(code)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.GbbqResp), nil
}

func (c *StConn) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	codes, err := c.GetStockCodeAll()
	if err != nil {
		return nil, err
	}
	gbbqs := map[string][]*protocol.Gbbq{}
	var resp *protocol.GbbqResp
	for _, code := range codes {
		for i := 0; i == 0 || i < DefaultRetry; i++ {
			resp, err = c.GetGbbq(code)
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
func (c *StConn) GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error) {
	normalized, err := standardSecurity(exchange, code)
	if err != nil {
		return nil, err
	}
	code = normalized
	r, err := c.SendFrame(protocol.MCompanyCat.Frame(exchange.Uint8(), code))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.CompanyCategory), nil
}

// GetCompanyContent 获取 F10 某分类的文本内容。
func (c *StConn) GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error) {
	normalized, err := standardSecurity(exchange, code)
	if err != nil {
		return "", err
	}
	code = normalized
	r, err := c.SendFrame(protocol.MCompanyContent.Frame(exchange.Uint8(), code, filename, start, length))
	if err != nil {
		return "", err
	}
	return r.(string), nil
}

// GetFinanceInfo 获取标的财务/基本面信息（流通股本/总股本/行业/地域/股东户数/财务）。
func (c *StConn) GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error) {
	normalized, err := standardSecurity(exchange, code)
	if err != nil {
		return nil, err
	}
	code = normalized
	f := protocol.MFinance.Frame(exchange.Uint8(), code)
	result, err := c.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.FinanceInfo), nil
}

// GetBlockFileRaw 下载通达信服务器文件（板块/配置）原始字节，分块拉取后拼接。
// 适用于二进制板块文件(block*.dat)与文本配置(tdxhy.cfg 等)。
func (c *StConn) GetBlockFileRaw(file string) ([]byte, error) {
	mr, err := c.SendFrame(protocol.MBlock.FrameMeta(file))
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
		data, err := c.fileChunk(file, start, n)
		if err != nil {
			return nil, err
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("文件 %s 下载不完整: 预期 %d 字节，收到 %d: %w", file, meta.Size, start, io.ErrUnexpectedEOF)
		}
		buf = append(buf, data...)
		start += uint32(len(data))
	}
	return buf, nil
}

// GetBlockData 下载并解析通达信板块文件（如 protocol.BlockFileGN 概念）→ 板块列表。
func (c *StConn) GetBlockData(file string) ([]*protocol.Block, error) {
	buf, err := c.GetBlockFileRaw(file)
	if err != nil {
		return nil, err
	}
	return protocol.ParseBlockFile(buf)
}

// GetReportFile 下载通达信服务器任意报表/数据文件（report file，指令 0x06B9）原始字节。
// 与 GetBlockFileRaw 共用同一传输帧，区别在于报表文件无 0x02C5 元信息预查文件大小，
// 故按 0x7530 块大小循环递增 offset 拉取，直到返回块短于请求块（末块）或为空时终止。
// 对齐 pytdx GetReportFile / mitdx get_report_file。
func (c *StConn) GetReportFile(file string) ([]byte, error) {
	const chunk = uint32(0x7530)
	var buf []byte
	start := uint32(0)
	for {
		data, err := c.fileChunk(file, start, chunk)
		if err != nil {
			return nil, err
		}
		buf = append(buf, data...)
		if uint32(len(data)) < chunk {
			break
		}
		if uint64(start)+uint64(len(data)) > uint64(^uint32(0)) {
			return nil, fmt.Errorf("文件 %s 偏移超出 uint32 上限", file)
		}
		start += uint32(len(data))
	}
	return buf, nil
}

func (c *StConn) fileChunk(file string, start, count uint32) ([]byte, error) {
	r, err := c.SendFrame(protocol.MBlock.FrameInfo(start, count, file))
	if err != nil {
		return nil, err
	}
	info, ok := r.(*protocol.BlockInfoResp)
	if !ok || info == nil {
		return nil, fmt.Errorf("文件 %s 分块响应类型错误", file)
	}
	if uint64(len(info.Data)) > uint64(count) {
		return nil, fmt.Errorf("文件 %s 分块过长: 预期最多 %d 字节，收到 %d", file, count, len(info.Data))
	}
	return info.Data, nil
}

// GetZHBFiles 下载板块/配置数据总包 zhb.zip(report file 0x06B9)并解压，返回 文件名→原始字节。
// zhb.zip 内含 tdxzs.cfg(板块指数代码)、tdxbk.cfg(概念板块)、incon.dat(行业分类)等配置文件。
func (c *StConn) GetZHBFiles() (map[string][]byte, error) {
	raw, err := c.GetReportFile(protocol.ReportZHB)
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

func (c *StConn) zhbFile(name string) ([]byte, error) {
	files, err := c.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	data, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("%s 中缺少 %s", protocol.ReportZHB, name)
	}
	return data, nil
}

// GetTdxZs 下载并解析板块指数配置 tdxzs.cfg(来自 zhb.zip) → 板块名↔指数代码(id) 列表。
func (c *StConn) GetTdxZs() ([]*protocol.TdxZs, error) {
	data, err := c.zhbFile(protocol.FileTdxZs)
	if err != nil {
		return nil, err
	}
	return protocol.ParseTdxZs(data), nil
}

// GetTdxBk 下载并解析 tdxbk.cfg(来自 zhb.zip) → 概念板块简称↔全称。
func (c *StConn) GetTdxBk() ([]*protocol.TdxBk, error) {
	data, err := c.zhbFile(protocol.FileTdxBk)
	if err != nil {
		return nil, err
	}
	return protocol.ParseTdxBk(data), nil
}

// GetBlockDataWithIndex 下载板块文件(block_*.dat)并按名称回填板块指数代码(id)。
// block 文件本身无 id，关联链: 板块名 →(tdxzs.cfg)→ id；直接未命中再经
// 简称 →(tdxbk.cfg)→ 全称 →(tdxzs.cfg)→ id 二次匹配。三个文件均来自 zhb.zip(仅下载一次)。
func (c *StConn) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	blocks, err := c.GetBlockData(file)
	if err != nil {
		return nil, err
	}
	files, err := c.GetZHBFiles()
	if err != nil {
		return nil, err
	}
	zs := protocol.ParseTdxZs(files[protocol.FileTdxZs])
	bk := protocol.ParseTdxBk(files[protocol.FileTdxBk])
	protocol.FillBlockIndexAlias(blocks, zs, bk)
	return blocks, nil
}

// GetTdxStat 下载并解析 tdxstat.cfg(来自 zhb.zip) → 全市场个股综合统计指标。
func (c *StConn) GetTdxStat() ([]*protocol.TdxStat, error) {
	data, err := c.zhbFile(protocol.FileTdxStat)
	if err != nil {
		return nil, err
	}
	return protocol.ParseTdxStat(data), nil
}

// GetTdxStat2 下载并解析 tdxstat2.cfg(来自 zhb.zip) → 全市场个股资金流向 + 板块归属。
// 其 BlockIndex 字段提供 股→板块指数代码(id) 的反向映射(见 protocol.StockBlockIndex)。
func (c *StConn) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	data, err := c.zhbFile(protocol.FileTdxStat2)
	if err != nil {
		return nil, err
	}
	return protocol.ParseTdxStat2(data), nil
}

// GetXgsg 下载并解析 xgsg.cfg(来自 zhb.zip) → 新股申购列表。
func (c *StConn) GetXgsg() ([]*protocol.TdxXgsg, error) {
	data, err := c.zhbFile(protocol.FileXgsg)
	if err != nil {
		return nil, err
	}
	return protocol.ParseXgsg(data), nil
}

// GetSpBlock 下载并解析 spblock.dat(来自 zhb.zip) → 专业板块成分列表。
//
// spblock.dat 承载 block_zs.dat 无法容纳的大型指数成分, 如
// 中证2000/中证1000/中证500/中证A500/国证2000/深证成指 等(block_*.dat 单板块成分上限 400)。
// 用 protocol.SpBlockByName 按名取单个板块。注意: 沪深300 不在此文件, 在 block_zs.dat(见 GetBlockData)。
func (c *StConn) GetSpBlock() ([]*protocol.SpBlock, error) {
	data, err := c.zhbFile(protocol.FileSpBlock)
	if err != nil {
		return nil, err
	}
	return protocol.ParseSpBlock(data), nil
}

// GetTdxHy 下载并解析 tdxhy.cfg → 每只股票的通达信/申万行业归属。
func (c *StConn) GetTdxHy() ([]*protocol.TdxHy, error) {
	buf, err := c.GetBlockFileRaw(protocol.FileTdxHy)
	if err != nil {
		return nil, err
	}
	return protocol.ParseTdxHy(buf), nil
}

// getStandardMinute 获取标准分时数据,todo 解析好像不对,先用历史数据
func (c *StConn) getStandardMinute(code string) (*protocol.MinuteResp, error) {
	// 实时分时解析存疑，移植后暂统一走历史分时（去除原 unreachable 实现以过 vet）。
	return c.getStandardHistoryMinute(time.Now().In(protocol.Location()).Format("20060102"), code)
}

// getStandardHistoryMinute 获取标准历史分时数据
func (c *StConn) getStandardHistoryMinute(date, code string) (*protocol.MinuteResp, error) {
	f, err := protocol.MHistoryMinute.Frame(date, code)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(*protocol.MinuteResp), nil
}

// getStandardTrade 获取标准分时交易详情,服务器最多返回1800条,count-start<=1800
func (c *StConn) getStandardTrade(code string, start, count uint16) (*protocol.TradeResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MTrade.Frame(code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f, protocol.TradeCache{
		Date: time.Now().In(protocol.Location()).Format("20060102"),
		Code: code,
	})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.TradeResp), nil
}

func (c *StConn) getStandardHistoryTrade(date, code string, start, count uint16) (*protocol.TradeResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MHistoryTrade.Frame(date, code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f, protocol.TradeCache{
		Date: date,
		Code: code,
	})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.TradeResp), nil
}

func (c *StConn) getStandardKline(category uint8, code string, start, count uint16, kind string) (*protocol.KlineResp, error) {
	code = protocol.AddPrefix(code)
	f, err := protocol.MKline.Frame(category, code, start, count)
	if err != nil {
		return nil, err
	}
	result, err := c.SendFrame(f, protocol.KlineCache{Type: category, Kind: kind, Code: code})
	if err != nil {
		return nil, err
	}
	return result.(*protocol.KlineResp), nil
}
func (*StConn) decode(f *protocol.Response, val any) (resp any, err error) {
	switch f.Type {

	case protocol.TypeConnect:

	case protocol.TypeHeart:

	case protocol.TypeCount:
		resp, err = protocol.MCount.Decode(f.Data)

	case protocol.TypeCode:
		resp, err = protocol.MCode.Decode(f.Data)

	case protocol.TypeQuote:
		resp, err = protocol.MQuote.Decode(f.Data)

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

	default:
		err = fmt.Errorf("通讯类型未解析:0x%X", f.Type)
	}
	return resp, err
}
func newStConn(core *conn) *StConn {
	c := &StConn{conn: core, quoteCodes: codecache.NewBase()}
	c.queries = &queries{source: c}
	return c
}
func (*StConn) protocol() protocol.MarketProtocol { return protocol.ProtocolStandard }

func (*StConn) GetQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error) {
	return nil, errors.New("市场行情列表需要扩展连接")
}

// DialDefault 默认连接方式
func DialDefault(op ...client.Option) (cli *StConn, err error) {
	op = append([]client.Option{WithRedial()}, op...)
	return DialHostsRange(Hosts, op...)
}

// Dial 与服务器建立连接
func Dial(addr string, op ...client.Option) (cli *StConn, err error) {
	return DialWith(NewTCPDial(withPort(addr, stPort)), op...)
}

// DialHosts 与服务器建立连接,多个服务器轮询,开启重试生效
func DialHosts(hosts []string, op ...client.Option) (cli *StConn, err error) {
	return DialWith(NewHostDial(hostAddrs(hosts, Hosts, stPort)), op...)
}

// DialHostsRandom 与服务器建立连接,多个服务器随机连接
func DialHostsRandom(hosts []string, op ...client.Option) (cli *StConn, err error) {
	return DialWith(NewRandomDial(hostAddrs(hosts, Hosts, stPort)), op...)
}

// DialHostsRange 遍历设置的服务地址进行连接,成功则结束遍历
func DialHostsRange(hosts []string, op ...client.Option) (cli *StConn, err error) {
	return DialWith(NewRangeDial(hostAddrs(hosts, Hosts, stPort)), op...)
}

// DialWith 与服务器建立连接
func DialWith(dial ios.DialFunc, op ...client.Option) (*StConn, error) {
	return dialSt(context.Background(), dial, op...)
}

func dialSt(ctx context.Context, dial ios.DialFunc, op ...client.Option) (*StConn, error) {
	c := newStConn(nil)
	core, err := dialWith(ctx, dial, 2*time.Second, protocol.MConnect.Frame, protocol.MHeart.Frame, c.decode, op...)
	if err != nil {
		return nil, err
	}
	c.conn = core
	return c, nil
}
