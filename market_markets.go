package tdx

import (
	"net"

	"github.com/injoyai/tdx/v2/protocol"
)

func standardMarkets() []Exchange {
	return []Exchange{
		{Market: uint16(protocol.ExchangeSZ), Name: "深圳", ShortName: "sz", Protocol: protocol.ProtocolStandard},
		{Market: uint16(protocol.ExchangeSH), Name: "上海", ShortName: "sh", Protocol: protocol.ProtocolStandard},
		{Market: uint16(protocol.ExchangeBJ), Name: "北京", ShortName: "bj", Protocol: protocol.ProtocolStandard},
	}
}

// GetMarkets 返回当前连接协议的市场表。
func (c *Conn) GetMarkets() ([]Exchange, error) {
	return c.protocolClient().GetMarkets()
}

// GetMarkets 返回内置标准市场，不建立扩展连接。
func (c *StandardClient) GetMarkets() ([]Exchange, error) {
	select {
	case <-c.Done():
		return nil, net.ErrClosed
	default:
	}
	if err := c.requireStandard(); err != nil {
		return nil, err
	}
	return standardMarkets(), nil
}

// GetMarkets 返回扩展服务器发现的市场；发现不代表查询路由已经接入。
func (c *ExtendedClient) GetMarkets() ([]Exchange, error) {
	select {
	case <-c.Done():
		return nil, net.ErrClosed
	default:
	}
	if err := c.protocolExchange(protocol.ExchangeHK, true); err != nil {
		return nil, err
	}
	r, err := c.getExtendedMarkets()
	if err != nil {
		return nil, err
	}
	out := make([]Exchange, 0, len(r))
	for j := range r {
		m := &r[j]
		out = append(out, Exchange{Market: m.Market, Name: m.Name, ShortName: m.ShortName, Protocol: protocol.ProtocolExtended, Extended: m})
	}
	return out, nil
}

// GetMarkets 合并内置标准市场和扩展服务器市场表；扩展发现失败时返回错误。
func (c *MarketClient) GetMarkets() ([]Exchange, error) {
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
