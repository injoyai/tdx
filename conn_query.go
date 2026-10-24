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
	GetKline(category uint8, code string, start, count uint16) (*KlineResp, error)
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

func protocolInstrument(code string, extended bool) (protocol.Instrument, protocol.MarketInfo, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return i, info, err
	}
	if (info.Protocol == protocol.ProtocolExtended) != extended {
		return i, info, errors.New("证券与当前连接协议不匹配；跨市场查询请使用 NewClient")
	}
	return i, info, nil
}

func protocolExchange(ex protocol.Exchange, extended bool) error {
	m, err := (protocol.Instrument{Exchange: ex}).Market()
	if err != nil {
		return err
	}
	info, _ := m.Info()
	if (info.Protocol == protocol.ProtocolExtended) != extended {
		return errors.New("证券与当前连接协议不匹配；跨市场查询请使用 NewClient")
	}
	return nil
}

func klineRequest(code string, category uint8, count uint16) (*KlineResp, protocol.MarketInfo, error) {
	i, info, err := instrumentInfo(code)
	if err != nil {
		return nil, info, err
	}
	if category > protocol.TypeKlineYear || count == 0 || count > 800 {
		return nil, info, errors.New("K线周期必须为0..11，单次数量必须为1..800")
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
	zone := ""
	if info.Protocol == protocol.ProtocolStandard {
		zone = protocol.Location().String()
	}
	return &KlineResp{Instrument: i, Category: category, VolumeUnit: unit, TimeZone: zone}, info, nil
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
	return []Exchange{
		{Market: uint16(protocol.ExchangeSZ), Name: "深圳", ShortName: "sz", Protocol: protocol.ProtocolStandard},
		{Market: uint16(protocol.ExchangeSH), Name: "上海", ShortName: "sh", Protocol: protocol.ProtocolStandard},
		{Market: uint16(protocol.ExchangeBJ), Name: "北京", ShortName: "bj", Protocol: protocol.ProtocolStandard},
	}
}

func codePageBounds(ex protocol.Exchange, start uint32, count uint16) error {
	if count == 0 || count > 1000 {
		return errors.New("代码分页 count 必须在 1 到 1000 之间")
	}
	if (ex == protocol.ExchangeSZ || ex == protocol.ExchangeSH) && start > 65535 {
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
	if i.Exchange > protocol.ExchangeBJ {
		return i, fmt.Errorf("市场 %s 不支持此标准专用接口", i.Exchange)
	}
	return i, nil
}

func standardSecurity(exchange protocol.Exchange, code string) (string, error) {
	if exchange > protocol.ExchangeBJ {
		return "", fmt.Errorf("市场 %d 不支持此标准专用接口", exchange)
	}
	i, err := standardInstrument((protocol.Instrument{Exchange: exchange, Code: code}).String())
	if err != nil {
		return "", err
	}
	return i.Code, nil
}
