package tdx

import (
	"errors"
	"fmt"
	"io"

	"github.com/injoyai/tdx/v2/protocol"
)

// GetCode 的 start 是指定市场内的偏移；标准协议每次最多返回 1000 项。
func (c *MarketClient) GetCode(ex protocol.Exchange, start uint32, count uint16) (*MarketCodeResp, error) {
	if count == 0 || count > 1000 {
		return nil, errors.New("代码分页 count 必须在 1 到 1000 之间")
	}
	if (ex == protocol.ExchangeSZ || ex == protocol.ExchangeSH) && start > 65535 {
		return nil, errors.New("分页偏移超过协议 uint16 上限")
	}
	link, err := c.connection(ex)
	if err != nil {
		return nil, err
	}
	var out *MarketCodeResp
	if ex == protocol.ExchangeSZ || ex == protocol.ExchangeSH {
		r, err := link.getStandardCode(ex, uint16(start))
		if err != nil {
			return nil, err
		}
		out = standardCodes(ex, r)
	} else {
		// shortcut: 扩展分页逐次扫描全局列表，频繁查询时改为显式快照分页。
		out, err = c.GetCodeAll(ex)
		if err != nil {
			return nil, err
		}
		if uint64(start) >= uint64(len(out.List)) {
			return &MarketCodeResp{List: []MarketCode{}}, nil
		}
		out.List = out.List[int(start):]
	}
	out.List = out.List[:min(int(count), len(out.List))]
	out.Count = len(out.List)
	return out, nil
}

func standardCodes(ex protocol.Exchange, r *protocol.CodeResp) *MarketCodeResp {
	out := &MarketCodeResp{Count: len(r.List), List: make([]MarketCode, 0, len(r.List))}
	for _, k := range r.List {
		out.List = append(out.List, MarketCode{Instrument: protocol.Instrument{Exchange: ex, Code: k.Code}, Name: k.Name, Standard: k})
	}
	return out
}

// GetCodeAll 返回指定协议市场的全部品种。扩展列表按全局序号分页再过滤，不能把市场编号当成偏移。
func (c *MarketClient) GetCodeAll(ex protocol.Exchange) (*MarketCodeResp, error) {
	link, err := c.connection(ex)
	if err != nil {
		return nil, err
	}
	out := &MarketCodeResp{List: []MarketCode{}}
	if ex == protocol.ExchangeSZ || ex == protocol.ExchangeSH || ex == protocol.ExchangeBJ {
		r, err := link.getStandardCodeAll(ex)
		if err != nil {
			return nil, err
		}
		return standardCodes(ex, r), nil
	} else {
		total, err := link.getExtendedCount()
		if err != nil {
			return nil, err
		}
		for start := 0; start < total; {
			count := min(800, total-start)
			items, err := link.getExtendedInstruments(uint32(start), uint16(count))
			if err != nil {
				return nil, err
			}
			if len(items) == 0 || len(items) > count {
				return nil, fmt.Errorf("扩展品种分页 %d 不完整: %w", start, io.ErrUnexpectedEOF)
			}
			for j := range items {
				k := &items[j]
				if k.Market == ex.Uint8() {
					out.List = append(out.List, MarketCode{Instrument: protocol.Instrument{Exchange: ex, Code: k.Code}, Name: k.Name, Extended: k})
				}
			}
			start += len(items)
		}
	}
	out.Count = len(out.List)
	return out, nil
}

// GetCount 统计指定市场；扩展协议的 ExCount 为所有市场总数，不能直接返回。
func (c *MarketClient) GetCount(ex protocol.Exchange) (int, error) {
	if ex == protocol.ExchangeSZ || ex == protocol.ExchangeSH {
		link, err := c.connection(ex)
		if err != nil {
			return 0, err
		}
		r, err := link.getStandardCount(ex)
		if err != nil {
			return 0, err
		}
		return int(r.Count), nil
	}
	r, err := c.GetCodeAll(ex)
	if err != nil {
		return 0, err
	}
	return r.Count, nil
}
