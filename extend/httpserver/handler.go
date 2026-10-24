package httpserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func respond[T any](s *Server, w http.ResponseWriter, r *http.Request, fetch func(tdx.Client) (T, error)) {
	var resp T
	pool, err := s.requestPool(r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	err = pool.DoContext(r.Context(), func(c tdx.Client) error {
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
	ex, err := queryExchange(r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (int, error) { return c.GetCount(ex) })
}

func (s *Server) handleCode(w http.ResponseWriter, r *http.Request) {
	ex, err := queryExchange(r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
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
	respond(s, w, r, func(c tdx.Client) (*tdx.CodeResp, error) { return c.GetCode(ex, start, count) })
}

func (s *Server) handleCodeAll(w http.ResponseWriter, r *http.Request) {
	ex, err := queryExchange(r)
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (*tdx.CodeResp, error) { return c.GetCodeAll(ex) })
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
	var groups [2][]string
	var positions [2][]int
	for pos, code := range codes {
		instrument, err := protocol.ParseInstrument(code)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		group := 0
		if instrument.Exchange > protocol.ExchangeBJ {
			group = 1
		}
		groups[group] = append(groups[group], code)
		positions[group] = append(positions[group], pos)
	}
	resp := make([]tdx.Quote, len(codes))
	for group, batch := range groups {
		if len(batch) == 0 {
			continue
		}
		pool := s.pool
		if group == 1 {
			pool = s.exPool
		}
		if pool == nil {
			respondErr(w, http.StatusBadRequest, "行情连接池未启用")
			return
		}
		err = pool.DoContext(r.Context(), func(c tdx.Client) error {
			quotes, err := c.GetQuote(batch...)
			if err != nil {
				return err
			}
			if len(quotes) != len(batch) {
				return fmt.Errorf("报价数量与请求不一致")
			}
			for i, q := range quotes {
				resp[positions[group][i]] = q
			}
			return nil
		})
		if err != nil {
			respondErr(w, http.StatusOK, err.Error())
			return
		}
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
	respond(s, w, r, func(c tdx.Client) (*tdx.MinuteResp, error) { return c.GetMinute(code) })
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
	respond(s, w, r, func(c tdx.Client) (*tdx.MinuteResp, error) { return c.GetHistoryMinute(date, code) })
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
	respond(s, w, r, func(c tdx.Client) (*tdx.TradeResp, error) { return c.GetTrade(code, start, count) })
}

func (s *Server) handleTradeAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	respond(s, w, r, func(c tdx.Client) (*tdx.TradeResp, error) { return c.GetTradeAll(code) })
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
	respond(s, w, r, func(c tdx.Client) (*tdx.TradeResp, error) { return c.GetHistoryTrade(date, code, start, count) })
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
	respond(s, w, r, func(c tdx.Client) (*tdx.TradeResp, error) { return c.GetHistoryTradeDay(date, code) })
}

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
		var resp *tdx.KlineResp
		pool, err := s.requestPool(r)
		if err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		err = pool.DoContext(r.Context(), func(c tdx.Client) error {
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
			resp = result
			return nil
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
