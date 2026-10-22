package httpserver

import (
	"net/http"
	"strings"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func standardCodePayload(r *tdx.CodeResp) *protocol.CodeResp {
	out := &protocol.CodeResp{Count: uint16(r.Count)}
	for _, k := range r.List {
		out.List = append(out.List, k.Standard)
	}
	return out
}

func respond[T any](s *Server, w http.ResponseWriter, r *http.Request, fetch func(tdx.Client) (T, error)) {
	var resp T
	err := s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		var err error
		resp, err = fetch(c)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- 代码/数量 ----

func (s *Server) handleCount(w http.ResponseWriter, r *http.Request) {
	exStr, err := queryStr(r, "exchange")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, err := parseExchange(exStr)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.CountResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		n, err := c.GetCount(ex)
		if err == nil {
			resp = &protocol.CountResp{Count: uint16(n)}
		}
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleCode(w http.ResponseWriter, r *http.Request) {
	exStr, err := queryStr(r, "exchange")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, err := parseExchange(exStr)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryUint16(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.CodeResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetCode(ex, uint32(start), 1000)
		if err == nil {
			resp = standardCodePayload(r)
		}
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleCodeAll(w http.ResponseWriter, r *http.Request) {
	exStr, err := queryStr(r, "exchange")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, err := parseExchange(exStr)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.CodeResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetCodeAll(ex)
		if err == nil {
			resp = standardCodePayload(r)
		}
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleStockCodeAll(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetStockCodeAll)
}

func (s *Server) handleETFCodeAll(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetETFCodeAll)
}

func (s *Server) handleIndexCodeAll(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetIndexCodeAll)
}

// ---- 行情/财务 ----

func (s *Server) handleQuote(w http.ResponseWriter, r *http.Request) {
	codesStr, err := queryStr(r, "codes")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	codes := strings.Split(codesStr, ",")
	var resp protocol.QuotesResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		quotes, err := c.GetQuote(codes...)
		if err != nil {
			return err
		}
		for _, q := range quotes {
			resp = append(resp, q.Standard)
		}
		return nil
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleCallAuction(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (*protocol.CallAuctionResp, error) {
		return c.GetCallAuction(code)
	})
}

func (s *Server) handleGbbq(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (*protocol.GbbqResp, error) {
		return c.GetGbbq(code)
	})
}

func (s *Server) handleFinanceInfo(w http.ResponseWriter, r *http.Request) {
	exStr, err := queryStr(r, "exchange")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, err := parseExchange(exStr)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (*protocol.FinanceInfo, error) {
		return c.GetFinanceInfo(ex, code)
	})
}

func (s *Server) handleCompanyCategory(w http.ResponseWriter, r *http.Request) {
	exStr, err := queryStr(r, "exchange")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, err := parseExchange(exStr)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) ([]protocol.CompanyCategory, error) {
		return c.GetCompanyCategory(ex, code)
	})
}

func (s *Server) handleCompanyContent(w http.ResponseWriter, r *http.Request) {
	exStr, err := queryStr(r, "exchange")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ex, err := parseExchange(exStr)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	filename, err := queryStr(r, "filename")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	start, err := queryUint32(r, "start")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	length, err := queryUint32(r, "length")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp string
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		resp, err = c.GetCompanyContent(ex, code, filename, start, length)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- 分时/成交 ----

func (s *Server) handleMinute(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.MinuteResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetMinute(code)
		if err != nil {
			return err
		}
		resp, err = r.StandardResponse()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleHistoryMinute(w http.ResponseWriter, r *http.Request) {
	date, err := queryStr(r, "date")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.MinuteResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetHistoryMinute(date, code)
		if err != nil {
			return err
		}
		resp, err = r.StandardResponse()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTrade(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.TradeResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetTrade(code, start, count)
		if err != nil {
			return err
		}
		resp, err = r.StandardResponse()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTradeAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.TradeResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetTradeAll(code)
		if err != nil {
			return err
		}
		resp, err = r.StandardResponse()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleHistoryTrade(w http.ResponseWriter, r *http.Request) {
	date, err := queryStr(r, "date")
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
	var resp *protocol.TradeResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetHistoryTrade(date, code, start, count)
		if err != nil {
			return err
		}
		resp, err = r.StandardResponse()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleHistoryTradeDay(w http.ResponseWriter, r *http.Request) {
	date, err := queryStr(r, "date")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.TradeResp
	err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
		r, err := c.GetHistoryTradeDay(date, code)
		if err != nil {
			return err
		}
		resp, err = r.StandardResponse()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// K线和指数共用参数校验与标准响应转换；指数解码由客户端按证券选择。
func (s *Server) handleKline(w http.ResponseWriter, r *http.Request) {
	typ, err := queryUint8(r, "type")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.klineHandler(typ, false)(w, r)
}

func (s *Server) handleKlineAll(w http.ResponseWriter, r *http.Request) {
	typ, err := queryUint8(r, "type")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.klineHandler(typ, true)(w, r)
}

func (s *Server) klineHandler(typ uint8, all bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code, err := queryStr(r, "code")
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		var start, count uint16
		if !all {
			start, err = queryUint16(r, "start")
			if err != nil {
				respondErr(w, http.StatusBadRequest, err.Error())
				return
			}
			count, err = queryUint16(r, "count")
			if err != nil {
				respondErr(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		var resp *protocol.KlineResp
		err = s.pool.DoContext(r.Context(), func(c tdx.Client) error {
			var result *tdx.KlineResp
			var err error
			if all {
				result, err = c.GetKlineAll(typ, code)
			} else {
				result, err = c.GetKline(typ, code, start, count)
			}
			if err != nil {
				return err
			}
			resp, err = result.StandardResponse()
			return err
		})
		if err != nil {
			respondErr(w, http.StatusOK, err.Error())
			return
		}
		respondOK(w, resp)
	}
}

// ---- 板块/报表 ----

func (s *Server) handleBlockData(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) ([]*protocol.Block, error) {
		return c.GetBlockData(file)
	})
}

func (s *Server) handleBlockDataWithIndex(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) ([]*protocol.Block, error) {
		return c.GetBlockDataWithIndex(file)
	})
}

func (s *Server) handleBlockFileRaw(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) ([]byte, error) {
		return c.GetBlockFileRaw(file)
	})
}

func (s *Server) handleReportFile(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) ([]byte, error) {
		return c.GetReportFile(file)
	})
}

func (s *Server) handleZHBFiles(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetZHBFiles)
}

func (s *Server) handleTdxZs(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetTdxZs)
}

func (s *Server) handleTdxBk(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetTdxBk)
}

func (s *Server) handleTdxStat(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetTdxStat)
}

func (s *Server) handleTdxStat2(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetTdxStat2)
}

func (s *Server) handleTdxXgsg(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetXgsg)
}

func (s *Server) handleTdxHy(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetTdxHy)
}

func (s *Server) handleSpBlock(w http.ResponseWriter, r *http.Request) {
	respond(s, w, r, tdx.Client.GetSpBlock)
}
