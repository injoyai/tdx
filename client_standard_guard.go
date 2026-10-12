package tdx

import (
	"errors"
	"fmt"

	"github.com/injoyai/tdx/v2/protocol"
)

func (c *Conn) requireStandard() error {
	if c.extended {
		return errors.New("此接口仅支持标准行情连接；跨市场使用 NewClient")
	}
	return nil
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
