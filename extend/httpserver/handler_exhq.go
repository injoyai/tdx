package httpserver

import (
	"net/http"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// ---- 扩展行情 ----

func (s *Server) handleExMarkets(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	var resp []protocol.ExMarket
	var err error
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExMarkets()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExCount(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	var resp int
	var err error
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExCount()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExInstruments(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	start, err := queryUint32(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := queryUint16(r, "count")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExInstrument
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExInstruments(start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExQuote(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.ExQuote
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExQuote(market, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExQuoteList(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	category, err := queryUint8(r, "category")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryUint16(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := queryUint16(r, "count")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExQuoteListItem
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExQuoteList(market, category, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExBars(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	category, err := queryUint8(r, "category")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryUint16(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := queryUint16(r, "count")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExKline
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExBars(category, market, code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExMinute(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExMinuteTick
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExMinute(market, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExHistMinute(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	date, err := queryUint32(r, "date")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExMinuteTick
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExHistMinute(market, code, date)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExTrade(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryUint16(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := queryUint16(r, "count")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExTradeTick
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExTrade(market, code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExHistTrade(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	date, err := queryUint32(r, "date")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryUint16(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := queryUint16(r, "count")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExTradeTick
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExHistTrade(market, code, date, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleExBarsRange(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	market, err := queryUint8(r, "market")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	date, err := queryUint32(r, "date")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	date2, err := queryUint32(r, "date2")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []protocol.ExRangeKline
	err = s.exPool.Do(func(c *tdx.Client) error {
		resp, err = c.ExBarsRange(market, code, date, date2)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}
