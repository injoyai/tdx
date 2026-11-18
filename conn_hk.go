package tdx

import (
	"errors"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/injoyai/tdx/v2/protocol"
)

type hkSnapshot struct {
	writer *frameWriter
	codes  protocol.HKInstruments
}

type hkState struct {
	gate     chan struct{}
	snapshot atomic.Pointer[hkSnapshot]
}

func (c *ExConn) instrument(code string) (protocol.Instrument, protocol.MarketInfo, error) {
	i, info, err := protocolInstrument(code, true)
	if err != nil {
		return i, info, err
	}
	symbol, shared := protocol.HKCode(code)
	if !shared {
		return i, info, nil
	}
	snapshot, err := c.hkSnapshot()
	if err != nil {
		return i, info, fmt.Errorf("hk.%s 无法读取通达信品种快照（候选市场 31、49）: %w", symbol, err)
	}
	ex, err := snapshot.Exchange(symbol)
	if err != nil {
		return i, info, err
	}
	i = protocol.Instrument{Exchange: ex, Code: symbol}
	info, _ = i.Info()
	return i, info, nil
}

// readInstruments serializes a refresh with an automatic Hong Kong lookup.
func (c *ExConn) readInstruments(refresh bool) ([]protocol.ExInstrument, protocol.HKInstruments, error) {
	ctx := c.requestContext()
	select {
	case c.hk.gate <- struct{}{}:
		defer func() { <-c.hk.gate }()
	case <-ctx.Done():
		return nil, protocol.HKInstruments{}, ctx.Err()
	case <-c.Done():
		return nil, protocol.HKInstruments{}, net.ErrClosed
	}
	writer := c.currentWriter()
	if previous := c.hk.snapshot.Load(); !refresh && previous != nil && previous.writer == writer {
		return nil, previous.codes, nil
	}
	items, err := c.scanInstruments()
	if err != nil {
		return nil, protocol.HKInstruments{}, err
	}
	if c.currentWriter() != writer {
		return nil, protocol.HKInstruments{}, errors.New("扩展行情连接在品种扫描期间变更")
	}
	if err := ctx.Err(); err != nil {
		return nil, protocol.HKInstruments{}, err
	}
	codes := protocol.NewHKInstruments(items)
	c.hk.snapshot.Store(&hkSnapshot{writer: writer, codes: codes})
	return items, codes, nil
}

func (c *ExConn) hkSnapshot() (protocol.HKInstruments, error) {
	if err := c.requestContext().Err(); err != nil {
		return protocol.HKInstruments{}, err
	}
	if snapshot := c.hk.snapshot.Load(); snapshot != nil && snapshot.writer == c.currentWriter() {
		return snapshot.codes, nil
	}
	_, codes, err := c.readInstruments(false)
	return codes, err
}
