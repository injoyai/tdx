package tdx

import (
	"errors"

	"github.com/injoyai/tdx/v2/protocol"
)

// queryClient 复用统一查询实现，但单连接入口不会自动建立另一类连接。
func (c *Conn) queryClient() *MarketClient {
	c.queryOnce.Do(func() {
		q := newMarketClient()
		unavailable := func() (*Conn, error) {
			return nil, errors.New("证券与当前连接协议不匹配；跨市场查询请使用 NewClient")
		}
		q.dial = [2]func() (*Conn, error){unavailable, unavailable}
		index := 0
		if c.extended {
			index = 1
		}
		q.links[index] = c
		c.queries = q
	})
	return c.queries
}

// GetQuote 标准/扩展五档报价，参数为带市场前缀的证券代码。
func (c *Conn) GetQuote(codes ...string) ([]MarketQuote, error) {
	return c.queryClient().GetQuote(codes...)
}

// GetKline 标准/扩展 K 线，自动选择解码分类及市场周期。
func (c *Conn) GetKline(category uint8, code string, start, count uint16) (*MarketKlineResp, error) {
	return c.queryClient().GetKline(category, code, start, count)
}

func (c *Conn) GetMinute(code string) (*MarketMinuteResp, error) {
	return c.queryClient().GetMinute(code)
}

func (c *Conn) GetHistoryMinute(date, code string) (*MarketMinuteResp, error) {
	return c.queryClient().GetHistoryMinute(date, code)
}

func (c *Conn) GetTrade(code string, start, count uint16) (*MarketTradeResp, error) {
	return c.queryClient().GetTrade(code, start, count)
}

func (c *Conn) GetHistoryTrade(date, code string, start, count uint16) (*MarketTradeResp, error) {
	return c.queryClient().GetHistoryTrade(date, code, start, count)
}

func (c *Conn) GetCount(exchange protocol.Exchange) (int, error) {
	return c.queryClient().GetCount(exchange)
}

func (c *Conn) GetCode(exchange protocol.Exchange, start uint32, count uint16) (*MarketCodeResp, error) {
	return c.queryClient().GetCode(exchange, start, count)
}

func (c *Conn) GetCodeAll(exchange protocol.Exchange) (*MarketCodeResp, error) {
	return c.queryClient().GetCodeAll(exchange)
}
