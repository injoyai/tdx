package httpserver

import (
	"fmt"
	"net/http"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func queryExchange(r *http.Request) (protocol.Exchange, error) {
	name, err := queryStr(r, "exchange")
	if err != nil {
		return 0, err
	}
	return protocol.ParseExchange(name)
}

// requestPool 使用查询的交易所选择连接池；标准专用接口仍由各 handler 校验。
func (s *Server) requestPool(r *http.Request) (*tdx.Pool, error) {
	ex := protocol.ExchangeSZ
	var err error
	if name := r.URL.Query().Get("exchange"); name != "" {
		ex, err = protocol.ParseExchange(name)
	} else if code := r.URL.Query().Get("code"); code != "" {
		var instrument protocol.Instrument
		instrument, err = protocol.ParseInstrument(code)
		ex = instrument.Exchange
	}
	if err != nil {
		return nil, err
	}
	pool := s.pool
	if ex > protocol.ExchangeBJ {
		pool = s.exPool
	}
	if pool == nil {
		return nil, fmt.Errorf("行情连接池未启用")
	}
	return pool, nil
}

func (s *Server) handleMarkets(w http.ResponseWriter, r *http.Request) {
	var markets []tdx.Exchange
	for _, pool := range []*tdx.Pool{s.pool, s.exPool} {
		if pool == nil {
			continue
		}
		err := pool.DoContext(r.Context(), func(c tdx.Client) error {
			list, err := c.GetMarkets()
			if err == nil {
				markets = append(markets, list...)
			}
			return err
		})
		if err != nil {
			respondErr(w, http.StatusOK, err.Error())
			return
		}
	}
	respondOK(w, markets)
}

func (s *Server) handleKlineRange(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryStr(r, "date")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	end, err := queryStr(r, "date2")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (*tdx.KlineResp, error) {
		return c.GetKlineRange(code, start, end)
	})
}
