package tdx

import (
	"errors"

	"github.com/injoyai/tdx/v2/protocol"
)

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
