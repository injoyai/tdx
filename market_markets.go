package tdx

import (
	"net"

	"github.com/injoyai/tdx/v2/protocol"
)

func standardMarkets() []MarketExchange {
	return []MarketExchange{
		{Market: uint16(protocol.ExchangeSZ), Name: "深圳", ShortName: "sz", Protocol: protocol.ProtocolStandard},
		{Market: uint16(protocol.ExchangeSH), Name: "上海", ShortName: "sh", Protocol: protocol.ProtocolStandard},
		{Market: uint16(protocol.ExchangeBJ), Name: "北京", ShortName: "bj", Protocol: protocol.ProtocolStandard},
	}
}

// GetMarkets 标准连接返回内置市场，扩展连接返回服务器发现的市场。
func (c *Client) GetMarkets() ([]MarketExchange, error) {
	select {
	case <-c.Done():
		return nil, net.ErrClosed
	default:
	}
	if !c.extended {
		return standardMarkets(), nil
	}
	r, err := c.getExtendedMarkets()
	if err != nil {
		return nil, err
	}
	out := make([]MarketExchange, 0, len(r))
	for j := range r {
		m := &r[j]
		out = append(out, MarketExchange{Market: m.Market, Name: m.Name, ShortName: m.ShortName, Protocol: protocol.ProtocolExtended, Extended: m})
	}
	return out, nil
}

// GetMarkets 合并内置标准市场和扩展服务器市场表；扩展发现失败时返回错误。
func (c *MarketClient) GetMarkets() ([]MarketExchange, error) {
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
