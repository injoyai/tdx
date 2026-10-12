package tdx

import (
	"reflect"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// Client 定义统一业务查询；具体连接和原始协议操作由 Conn 提供。
// Close 结束整个客户端生命周期，关闭后不再重连或建立新连接。
type Client interface {
	Close() error
	GetBlockData(file string) ([]*protocol.Block, error)
	GetBlockDataWithIndex(file string) ([]*protocol.Block, error)
	GetBlockFileRaw(file string) ([]byte, error)
	GetCallAuction(code string) (*protocol.CallAuctionResp, error)
	GetCode(exchange protocol.Exchange, start uint32, count uint16) (*MarketCodeResp, error)
	GetCodeAll(exchange protocol.Exchange) (*MarketCodeResp, error)
	GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error)
	GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error)
	GetCount(exchange protocol.Exchange) (int, error)
	GetETFCodeAll() ([]string, error)
	GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error)
	GetGbbq(code string) (*protocol.GbbqResp, error)
	GetGbbqAll() (map[string][]*protocol.Gbbq, error)
	GetHistoryMinute(date, code string) (*MarketMinuteResp, error)
	GetHistoryTrade(date, code string, start, count uint16) (*MarketTradeResp, error)
	GetHistoryTradeBefore(code string, calendar TradingCalendar, before time.Time) (*MarketTradeResp, error)
	GetHistoryTradeDay(date, code string) (*MarketTradeResp, error)
	GetHistoryTradeFull(code string, calendar TradingCalendar) (*MarketTradeResp, error)
	GetIndexCodeAll() ([]string, error)
	GetKline(category uint8, code string, start, count uint16) (*MarketKlineResp, error)
	GetKline15Minute(code string, start, count uint16) (*MarketKlineResp, error)
	GetKline15MinuteAll(code string) (*MarketKlineResp, error)
	GetKline15MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKline30Minute(code string, start, count uint16) (*MarketKlineResp, error)
	GetKline30MinuteAll(code string) (*MarketKlineResp, error)
	GetKline30MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKline5Minute(code string, start, count uint16) (*MarketKlineResp, error)
	GetKline5MinuteAll(code string) (*MarketKlineResp, error)
	GetKline5MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKline60Minute(code string, start, count uint16) (*MarketKlineResp, error)
	GetKline60MinuteAll(code string) (*MarketKlineResp, error)
	GetKline60MinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineAll(category uint8, code string) (*MarketKlineResp, error)
	GetKlineDay(code string, start, count uint16) (*MarketKlineResp, error)
	GetKlineDayAll(code string) (*MarketKlineResp, error)
	GetKlineDayUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineMinute(code string, start, count uint16) (*MarketKlineResp, error)
	GetKlineMinute241Until(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineMinuteAll(code string) (*MarketKlineResp, error)
	GetKlineMinuteUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineMonth(code string, start, count uint16) (*MarketKlineResp, error)
	GetKlineMonthAll(code string) (*MarketKlineResp, error)
	GetKlineMonthUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineQuarter(code string, start, count uint16) (*MarketKlineResp, error)
	GetKlineQuarterAll(code string) (*MarketKlineResp, error)
	GetKlineQuarterUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineRange(code, startDate, endDate string) (*MarketKlineResp, error)
	GetKlineUntil(category uint8, code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineWeek(code string, start, count uint16) (*MarketKlineResp, error)
	GetKlineWeekAll(code string) (*MarketKlineResp, error)
	GetKlineWeekUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetKlineYear(code string, start, count uint16) (*MarketKlineResp, error)
	GetKlineYearAll(code string) (*MarketKlineResp, error)
	GetKlineYearUntil(code string, stop func(MarketKline) bool) (*MarketKlineResp, error)
	GetMarkets() ([]MarketExchange, error)
	GetMinute(code string) (*MarketMinuteResp, error)
	GetQuote(codes ...string) ([]MarketQuote, error)
	GetQuoteList(market, category uint8, start, count uint16) ([]protocol.ExQuoteListItem, error)
	GetReportFile(file string) ([]byte, error)
	GetSpBlock() ([]*protocol.SpBlock, error)
	GetStockCodeAll() ([]string, error)
	GetTdxBk() ([]*protocol.TdxBk, error)
	GetTdxHy() ([]*protocol.TdxHy, error)
	GetTdxStat() ([]*protocol.TdxStat, error)
	GetTdxStat2() ([]*protocol.TdxStat2, error)
	GetTdxZs() ([]*protocol.TdxZs, error)
	GetTrade(code string, start, count uint16) (*MarketTradeResp, error)
	GetTradeAll(code string) (*MarketTradeResp, error)
	GetXgsg() ([]*protocol.TdxXgsg, error)
	GetZHBFiles() (map[string][]byte, error)
	SetTimeout(t time.Duration)
}

var _ Client = (*Conn)(nil)
var _ Client = (*MarketClient)(nil)

// 保留旧指针参数传 nil 的行为，避免接口包装空指针后被当成有效客户端。
func clientOrNil(c Client) Client {
	if c != nil && reflect.ValueOf(c).Kind() == reflect.Pointer && reflect.ValueOf(c).IsNil() {
		return nil
	}
	return c
}
