package model

import (
	"fmt"
	"strings"

	"github.com/injoyai/tdx/v2/protocol"
)

// Code is a security identified by its market and canonical storage code.
type Code struct {
	Market protocol.Market
	Code   string // standard: sz000001; extended: 00700, AAPL, IF2609
	Name   string
}

// Instrument validates a canonical code in a built-in market.
func (c Code) Instrument() (protocol.Instrument, error) {
	ex, known := c.Market.Exchange()
	if !known {
		return protocol.Instrument{}, fmt.Errorf("未定义的行情市场 %s", c.Market)
	}
	body := c.Code
	info, _ := c.Market.Info()
	if info.Protocol == protocol.ProtocolStandard {
		body = strings.TrimPrefix(body, ex.String())
	}
	i, err := c.Market.ParseInstrument(body)
	if err != nil {
		return i, err
	}
	if i.StorageCode() != c.Code {
		return protocol.Instrument{}, fmt.Errorf("市场 %s 的代码 %q 不是规范形式", c.Market, c.Code)
	}
	return i, nil
}

// Key returns the security's display key.
func (c Code) Key() string {
	if i, err := c.Instrument(); err == nil {
		return i.String()
	}
	return c.Market.String() + "." + c.Code
}
