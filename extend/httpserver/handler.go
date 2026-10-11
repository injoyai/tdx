package httpserver

import (
	"net/http"
	"strings"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetCount(ex)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetCode(ex, start)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetCodeAll(ex)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleStockCodeAll(w http.ResponseWriter, r *http.Request) {
	var resp []string
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetStockCodeAll()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleETFCodeAll(w http.ResponseWriter, r *http.Request) {
	var resp []string
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetETFCodeAll()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexCodeAll(w http.ResponseWriter, r *http.Request) {
	var resp []string
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexCodeAll()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetQuote(codes...)
		return err
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
	var resp *protocol.CallAuctionResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetCallAuction(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleGbbq(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.GbbqResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetGbbq(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
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
	var resp *protocol.FinanceInfo
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetFinanceInfo(ex, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
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
	var resp []protocol.CompanyCategory
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetCompanyCategory(ex, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
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
	err = s.pool.Do(func(c *tdx.Client) error {
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetMinute(code)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetHistoryMinute(date, code)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTrade(code, start, count)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTradeAll(code)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetHistoryTrade(date, code, start, count)
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
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetHistoryTradeDay(date, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- K线(股票) - 通用 ----

func (s *Server) handleKline(w http.ResponseWriter, r *http.Request) {
	typ, err := queryUint8(r, "type")
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline(typ, code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineAll(w http.ResponseWriter, r *http.Request) {
	typ, err := queryUint8(r, "type")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineAll(typ, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- K线(股票) - 各周期 ----

func (s *Server) handleKlineMinute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineMinute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineMinuteAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineMinuteAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline5Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline5Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline5MinuteAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline5MinuteAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline15Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline15Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline15MinuteAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline15MinuteAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline30Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline30Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline30MinuteAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline30MinuteAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline60Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline60Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKline60MinuteAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKline60MinuteAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineDay(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineDay(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineDayAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineDayAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineWeek(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineWeek(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineWeekAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineWeekAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineMonth(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineMonth(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineMonthAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineMonthAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineQuarter(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineQuarter(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineQuarterAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineQuarterAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineYear(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineYear(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleKlineYearAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetKlineYearAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- 指数K线 - 通用 ----

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	typ, err := queryUint8(r, "type")
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndex(typ, code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexAll(w http.ResponseWriter, r *http.Request) {
	typ, err := queryUint8(r, "type")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexAll(typ, code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- 指数K线 - 各周期 ----

func (s *Server) handleIndexMinute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexMinute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndex5Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndex5Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndex15Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndex15Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndex30Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndex30Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndex60Minute(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndex60Minute(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexDay(w http.ResponseWriter, r *http.Request) {
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
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexDay(code, start, count)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- 指数K线 - All 变体 ----

func (s *Server) handleIndexDayAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexDayAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexWeekAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexWeekAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexMonthAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexMonthAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexQuarterAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexQuarterAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleIndexYearAll(w http.ResponseWriter, r *http.Request) {
	code, err := queryStr(r, "code")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp *protocol.KlineResp
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetIndexYearAll(code)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

// ---- 板块/报表 ----

func (s *Server) handleBlockData(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []*protocol.Block
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetBlockData(file)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleBlockDataWithIndex(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []*protocol.Block
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetBlockDataWithIndex(file)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleBlockFileRaw(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []byte
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetBlockFileRaw(file)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleReportFile(w http.ResponseWriter, r *http.Request) {
	file, err := queryStr(r, "file")
	if err != nil {
		respondErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var resp []byte
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetReportFile(file)
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleZHBFiles(w http.ResponseWriter, r *http.Request) {
	var resp map[string][]byte
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetZHBFiles()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTdxZs(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.TdxZs
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTdxZs()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTdxBk(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.TdxBk
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTdxBk()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTdxStat(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.TdxStat
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTdxStat()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTdxStat2(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.TdxStat2
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTdxStat2()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTdxXgsg(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.TdxXgsg
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetXgsg()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleTdxHy(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.TdxHy
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetTdxHy()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}

func (s *Server) handleSpBlock(w http.ResponseWriter, r *http.Request) {
	var resp []*protocol.SpBlock
	var err error
	err = s.pool.Do(func(c *tdx.Client) error {
		resp, err = c.GetSpBlock()
		return err
	})
	if err != nil {
		respondErr(w, http.StatusOK, err.Error())
		return
	}
	respondOK(w, resp)
}
