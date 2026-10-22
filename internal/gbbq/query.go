// Package gbbq shares record calculations between local and HTTP metadata caches.
package gbbq

import (
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func EquityAt(records []*protocol.Gbbq, at time.Time) *protocol.Equity {
	for i := len(records) - 1; i >= 0; i-- {
		v := records[i]
		if !v.IsEquity() {
			continue
		}
		y, m, d := v.Time.Date()
		// 记录中的 15:00 不影响股本在所属时区当日零点生效。
		day := time.Date(y, m, d, 0, 0, 0, 0, v.Time.Location())
		if at.Unix() >= day.Unix() {
			return v.Equity()
		}
	}
	return nil
}

func XRXDs(records []*protocol.Gbbq) protocol.XRXDs {
	res := protocol.XRXDs{}
	for _, v := range records {
		if v.IsXRXD() {
			res = append(res, v.XRXD())
		}
	}
	return res
}

func Turnover(equity *protocol.Equity, volume int64) float64 {
	if equity == nil {
		return 0
	}
	return equity.Turnover(volume)
}
