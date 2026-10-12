package httpserver

import (
	"net/http"
	"strconv"

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
		r, err := c.GetMarkets()
		if err != nil {
			return err
		}
		resp = make([]protocol.ExMarket, 0, len(r))
		for _, m := range r {
			resp = append(resp, *m.Extended)
		}
		return nil
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
		// 此历史路由返回扩展全局总数，不能改为单市场 GetCount。
		r, err := c.SendFrame(protocol.MEx.FrameCount())
		if err == nil {
			resp = r.(int)
		}
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
		// 保留此路由的全局分页偏移。
		r, err := c.SendFrame(protocol.MEx.FrameInstrument(start, count))
		if err == nil {
			resp = r.([]protocol.ExInstrument)
		}
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
		r, err := c.GetQuote((protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String())
		if err != nil {
			return err
		}
		resp = r[0].Extended
		return nil
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
		resp, err = c.GetQuoteList(market, category, start, count)
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
	resp := []protocol.ExKline{}
	err = s.exPool.Do(func(c *tdx.Client) error {
		r, err := c.GetKline(category, (protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String(), start, count)
		if err != nil {
			return err
		}
		for _, k := range r.List {
			resp = append(resp, *k.Extended)
		}
		return nil
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
	resp := []protocol.ExMinuteTick{}
	err = s.exPool.Do(func(c *tdx.Client) error {
		r, err := c.GetMinute((protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String())
		if err != nil {
			return err
		}
		for _, k := range r.List {
			resp = append(resp, *k.Extended)
		}
		return nil
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
	resp := []protocol.ExMinuteTick{}
	err = s.exPool.Do(func(c *tdx.Client) error {
		r, err := c.GetHistoryMinute(strconv.FormatUint(uint64(date), 10), (protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String())
		if err != nil {
			return err
		}
		for _, k := range r.List {
			resp = append(resp, *k.Extended)
		}
		return nil
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
	resp := []protocol.ExTradeTick{}
	err = s.exPool.Do(func(c *tdx.Client) error {
		r, err := c.GetTrade((protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String(), start, count)
		if err != nil {
			return err
		}
		for _, k := range r.List {
			resp = append(resp, *k.Extended)
		}
		return nil
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
	resp := []protocol.ExTradeTick{}
	err = s.exPool.Do(func(c *tdx.Client) error {
		r, err := c.GetHistoryTrade(strconv.FormatUint(uint64(date), 10), (protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String(), start, count)
		if err != nil {
			return err
		}
		for _, k := range r.List {
			resp = append(resp, *k.Extended)
		}
		return nil
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
		r, err := c.GetKlineRange((protocol.Instrument{Exchange: protocol.Exchange(market), Code: code}).String(), strconv.FormatUint(uint64(date), 10), strconv.FormatUint(uint64(date2), 10))
		if err != nil {
			return err
		}
		resp = make([]protocol.ExRangeKline, 0, r.Count)
		for _, k := range r.List {
			resp = append(resp, *k.RangeExtended)
		}
		return nil
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}
