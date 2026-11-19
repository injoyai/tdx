// Package market registers the built-in standard and extended pull adapters.
package market

import (
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

func init() {
	for _, m := range protocol.Markets() {
		ex, _ := m.Exchange()
		info, _ := ex.Route()
		switch info.Protocol {
		case protocol.ProtocolStandard:
			pull.RegisterMarket(&stdUnit{name: m})
		case protocol.ProtocolExtended:
			pull.RegisterMarket(&exUnit{name: m})
		}
	}
}
