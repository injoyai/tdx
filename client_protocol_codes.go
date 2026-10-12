package tdx

import (
	"fmt"
	"io"

	"github.com/injoyai/tdx/v2/protocol"
)

// GetCode 获取标准市场代码的一页；北交所使用完整报表列表后分页。
func (c *StandardClient) GetCode(ex protocol.Exchange, start uint32, count uint16) (*CodeResp, error) {
	if err := codePageBounds(ex, start, count); err != nil {
		return nil, err
	}
	if err := c.protocolExchange(ex, false); err != nil {
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
func (c *StandardClient) GetCodeAll(ex protocol.Exchange) (*CodeResp, error) {
	if err := c.protocolExchange(ex, false); err != nil {
		return nil, err
	}
	r, err := c.getStandardCodeAll(ex)
	if err != nil {
		return nil, err
	}
	return standardCodes(ex, r), nil
}

// GetCount 返回标准市场品种数量；北交所按报表列表统计。
func (c *StandardClient) GetCount(ex protocol.Exchange) (int, error) {
	if err := c.protocolExchange(ex, false); err != nil {
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

// GetCode 按扩展市场内偏移分页，不将市场编号当作全局品种偏移。
func (c *ExtendedClient) GetCode(ex protocol.Exchange, start uint32, count uint16) (*CodeResp, error) {
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

func codePage(out *CodeResp, start uint32, count uint16) *CodeResp {
	if uint64(start) >= uint64(len(out.List)) {
		return &CodeResp{List: []Code{}}
	}
	out.List = out.List[int(start):]
	out.List = out.List[:min(int(count), len(out.List))]
	out.Count = len(out.List)
	return out
}

// GetCodeAll 扫描扩展全局品种表，再按指定市场过滤；不增加跨查询缓存。
func (c *ExtendedClient) GetCodeAll(ex protocol.Exchange) (*CodeResp, error) {
	if err := c.protocolExchange(ex, true); err != nil {
		return nil, err
	}
	total, err := c.getExtendedCount()
	if err != nil {
		return nil, err
	}
	out := &CodeResp{List: []Code{}}
	for start := 0; start < total; {
		count := min(800, total-start)
		items, err := c.getExtendedInstruments(uint32(start), uint16(count))
		if err != nil {
			return nil, err
		}
		if len(items) == 0 || len(items) > count {
			return nil, fmt.Errorf("扩展品种分页 %d 不完整: %w", start, io.ErrUnexpectedEOF)
		}
		for j := range items {
			k := &items[j]
			if k.Market == ex.Uint8() {
				out.List = append(out.List, Code{Instrument: protocol.Instrument{Exchange: ex, Code: k.Code}, Name: k.Name, Extended: k})
			}
		}
		start += len(items)
	}
	out.Count = len(out.List)
	return out, nil
}

// GetCount 返回扩展单市场数量，而非服务器全市场总数。
func (c *ExtendedClient) GetCount(ex protocol.Exchange) (int, error) {
	r, err := c.GetCodeAll(ex)
	if err != nil {
		return 0, err
	}
	return r.Count, nil
}
