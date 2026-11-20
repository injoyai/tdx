package tdx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// querySource 提供分页与历史查询依赖的基本操作。
type querySource interface {
	KlineReader
	instrument(code string) (protocol.Instrument, protocol.InstrumentRules, error)
	minutePage(date, code string) (*MinuteResp, error)
	tradePage(date, code string, start, count uint16) (*TradeResp, error)
}

// protocolClient 只由绑定一种协议的具体连接实现。
type protocolClient interface {
	Client
	querySource
	protocol() protocol.MarketProtocol
	Done() <-chan struct{}
}

// queries 共享业务分页和日期检查，不持有网络连接。
type queries struct{ source querySource }

func protocolInstrument(code string, extended bool) (protocol.Instrument, protocol.InstrumentRules, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return i, info, err
	}
	if (info.Protocol == protocol.ProtocolExtended) != extended {
		return i, info, protocolMismatch()
	}
	return i, info, nil
}

func protocolMismatch() error {
	return errors.New("证券与当前连接协议不匹配；跨市场查询请使用 NewClient")
}

func protocolExchange(ex protocol.Exchange, extended bool) error {
	info, ok := ex.Route()
	if !ok {
		return fmt.Errorf("行情市场 %d 尚未接入统一 Client", ex)
	}
	if (info.Protocol == protocol.ProtocolExtended) != extended {
		return protocolMismatch()
	}
	return nil
}

// quoteListRoute 校验批量报价请求的市场与类别，两个公开入口共用同一顺序和错误文案。
func quoteListRoute(market, category uint8) (protocol.Exchange, error) {
	ex := protocol.Exchange(market)
	route, ok := ex.Route()
	if !ok {
		return ex, fmt.Errorf("行情市场 %d 尚未接入统一 Client", market)
	}
	if route.Protocol != protocol.ProtocolExtended {
		return ex, errors.New("市场行情列表需要扩展市场")
	}
	if route.QuoteListCategory == 0 {
		return ex, fmt.Errorf("行情市场 %d 未声明批量报价类别", market)
	}
	if category != route.QuoteListCategory {
		return ex, fmt.Errorf("行情市场 %d 的报价类别为 %d，不接受类别 %d", market, route.QuoteListCategory, category)
	}
	return ex, nil
}

func klineBounds(category uint8, count uint16) error {
	if category > protocol.TypeKlineYear || count == 0 || count > 800 {
		return errors.New("K线周期必须为0..11，单次数量必须为1..800")
	}
	return nil
}

func codePage(out *CodeResp, start uint32, count uint16) *CodeResp {
	if uint64(start) >= uint64(len(out.List)) {
		return &CodeResp{List: []Code{}}
	}
	out.List = out.List[int(start):]
	out.List = out.List[:min(int(count), len(out.List))]
	out.Count = len(out.List)
	return out
}

func klineRangeDates(startDate, endDate string) (uint32, uint32, string, string, error) {
	start, err := parseMarketDate(startDate)
	if err != nil {
		return 0, 0, "", "", err
	}
	end, err := parseMarketDate(endDate)
	if err != nil {
		return 0, 0, "", "", err
	}
	if start > end {
		return 0, 0, "", "", errors.New("起始日期不能晚于结束日期")
	}
	from, _ := time.Parse("20060102", startDate)
	to, _ := time.Parse("20060102", endDate)
	return start, end, from.Format(time.DateOnly), to.Format(time.DateOnly), nil
}

func filterKlineRange(out *KlineResp, fromDay, toDay string) *KlineResp {
	items := make([]Kline, 0, len(out.List))
	for _, k := range out.List {
		day := strings.SplitN(k.Time, " ", 2)[0]
		if day >= fromDay && day <= toDay {
			items = append(items, k)
		}
	}
	out.List, out.Count = items, len(items)
	return out
}

func standardMarkets() []Exchange {
	exchanges := protocol.StandardExchanges()
	out := make([]Exchange, 0, len(exchanges))
	for _, ex := range exchanges {
		out = append(out, Exchange{
			Market: uint16(ex), Name: ex.Name(), ShortName: ex.String(), Protocol: protocol.ProtocolStandard,
		})
	}
	return out
}

func codePageBounds(ex protocol.Exchange, start uint32, count uint16) error {
	if count == 0 || count > 1000 {
		return errors.New("代码分页 count 必须在 1 到 1000 之间")
	}
	if route, ok := ex.Route(); ok && route.Protocol == protocol.ProtocolStandard && route.CodeSource != protocol.CodeListZHBReport && start > 65535 {
		return errors.New("分页偏移超过协议 uint16 上限")
	}
	return nil
}

func standardCodes(ex protocol.Exchange, r *protocol.CodeResp) *CodeResp {
	out := &CodeResp{Count: len(r.List), List: make([]Code, 0, len(r.List))}
	for _, k := range r.List {
		out.List = append(out.List, Code{Instrument: protocol.Instrument{Exchange: ex, Code: k.Code}, Name: k.Name, Standard: k})
	}
	return out
}

func parseMarketDate(date string) (uint32, error) {
	if _, err := time.Parse("20060102", date); err != nil {
		return 0, fmt.Errorf("日期必须为有效 YYYYMMDD: %w", err)
	}
	n, err := strconv.ParseUint(date, 10, 32)
	return uint32(n), err
}

func standardInstrument(code string) (protocol.Instrument, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return i, err
	}
	if route, ok := i.Exchange.Route(); !ok || route.Protocol != protocol.ProtocolStandard {
		return i, fmt.Errorf("市场 %s 不支持此标准专用接口", i.Exchange)
	}
	return i, nil
}
