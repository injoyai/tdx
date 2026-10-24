package httpserver

import (
	"fmt"
	"net/http"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// ---- 扩展行情 ----

func (s *Server) handleExCount(w http.ResponseWriter, r *http.Request) {
	if s.exPool == nil {
		respondErr(w, http.StatusNotFound, "扩展行情未启用")
		return
	}
	var resp int
	var err error
	err = s.exPool.DoContext(r.Context(), func(c tdx.Client) error {
		ex, ok := c.(*tdx.ExConn)
		if !ok || ex == nil {
			return fmt.Errorf("需要扩展行情连接")
		}
		// 原始扩展协议的全市场总数，不是某交易所的 GetCount。
		r, err := ex.SendFrame(protocol.MEx.FrameCount())
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
	err = s.exPool.DoContext(r.Context(), func(c tdx.Client) error {
		ex, ok := c.(*tdx.ExConn)
		if !ok || ex == nil {
			return fmt.Errorf("需要扩展行情连接")
		}
		// 保留此路由的全局分页偏移。
		r, err := ex.SendFrame(protocol.MEx.FrameInstrument(start, count))
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
	var resp []protocol.QuoteListItem
	err = s.exPool.DoContext(r.Context(), func(c tdx.Client) error {
		resp, err = c.GetQuoteList(market, category, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}
