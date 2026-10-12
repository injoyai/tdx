package tdx

import (
	"reflect"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// Client 定义标准与扩展行情共用的业务接口。
// NewClient 返回按市场按需建连的 MarketClient；Dial* 返回绑定单种协议的 Conn，
// 查询与该连接协议不匹配的品种会报错，不会自动建立另一类连接。
// StandardClient 和 ExtendedClient 分别实现对应协议业务，复用同一 Conn 的生命周期；
// 可用 NewStandardClient/NewExtendedClient 包装现有连接，也可继续直接使用 Conn。
//
// code 推荐使用带市场前缀的证券代码，如 sh600000、hk00700、usAAPL、cffIF2609；
// 显式传入 exchange 的方法使用该市场的代码主体。date 使用有效 YYYYMMDD。
// 统一结果的价格按品种原币种/报价单位表示，成交量以 VolumeUnit 为准；
// Standard、Extended 和 RangeExtended 保留原始协议数据，未知尺度不推测换算。
// 标准专用方法仍保留原 protocol 返回类型及单位，不支持扩展品种。
type Client interface {
	// Close 结束整个客户端生命周期，关闭后不再重连或建立新连接。
	Close() error
	// GetBlockData 下载并解析标准板块文件，如 protocol.BlockFileGN。
	GetBlockData(file string) ([]*protocol.Block, error)
	// GetBlockDataWithIndex 获取标准板块，并按名称回填可匹配的板块指数代码。
	GetBlockDataWithIndex(file string) ([]*protocol.Block, error)
	// GetBlockFileRaw 下载标准板块或行业配置文件的原始字节。
	GetBlockFileRaw(file string) ([]byte, error)
	// GetCallAuction 获取标准证券的集合竞价数据。
	GetCallAuction(code string) (*protocol.CallAuctionResp, error)
	// GetCode 按指定市场内的偏移分页；start 从 0 开始，count 为 1..1000。
	// 扩展市场先扫描全局品种列表，再过滤和分页；沪深 start 不得超过 65535。
	GetCode(exchange protocol.Exchange, start uint32, count uint16) (*CodeResp, error)
	// GetCodeAll 获取指定市场的完整品种列表；北交所使用报表来源。
	GetCodeAll(exchange protocol.Exchange) (*CodeResp, error)
	// GetCompanyCategory 获取标准证券的 F10 分类目录。
	GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error)
	// GetCompanyContent 按目录中的文件名、字节偏移和长度获取标准 F10 文本。
	GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error)
	// GetCount 返回指定市场的品种数，扩展市场的数量按完整列表过滤统计。
	GetCount(exchange protocol.Exchange) (int, error)
	// GetETFCodeAll 获取沪深 ETF/LOF 代码，返回值带市场前缀。
	GetETFCodeAll() ([]string, error)
	// GetFinanceInfo 获取标准证券财务信息，字段单位沿用 protocol.FinanceInfo。
	GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error)
	// GetGbbq 获取标准证券的股本、除权除息记录。
	GetGbbq(code string) (*protocol.GbbqResp, error)
	// GetGbbqAll 逐只查询标准股票的股本、除权除息记录，以带前缀代码为键。
	GetGbbqAll() (map[string][]*protocol.Gbbq, error)
	// GetHistoryMinute 获取指定日期的分时数据，date 格式为 YYYYMMDD。
	GetHistoryMinute(date, code string) (*MinuteResp, error)
	// GetHistoryTrade 获取指定日期的一页分笔；start 从最新端偏移，count 为 1..2000。
	// 查询前检查该日期日线，无对应日线时返回空列表，检查失败则返回错误。
	GetHistoryTrade(date, code string, start, count uint16) (*TradeResp, error)
	// GetHistoryTradeBefore 从最早可用月线所在月份起，按日历读取到 before 的整日分笔。
	// calendar 必须匹配证券市场且非 nil；结束边界沿用日历的 Range 语义。
	GetHistoryTradeBefore(code string, calendar TradingCalendar, before time.Time) (*TradeResp, error)
	// GetHistoryTradeDay 分页读取指定日期全部可用分笔；无对应日线时返回空列表。
	GetHistoryTradeDay(date, code string) (*TradeResp, error)
	// GetHistoryTradeFull 按指定市场日历读取截至当前时间的可用历史分笔。
	GetHistoryTradeFull(code string, calendar TradingCalendar) (*TradeResp, error)
	// GetIndexCodeAll 获取沪深指数代码及 bj899050，返回值带市场前缀。
	GetIndexCodeAll() ([]string, error)
	// GetKline 使用 protocol.TypeKline* 周期；start 从最新端偏移，count 为 1..800。
	// 日线和一分钟别名按市场映射，响应 Category 保留实际协议周期。
	GetKline(category uint8, code string, start, count uint16) (*KlineResp, error)
	// GetKline15Minute 获取十五分钟 K 线的一页数据，分页参数同 GetKline。
	GetKline15Minute(code string, start, count uint16) (*KlineResp, error)
	// GetKline15MinuteAll 分页读取全部可用十五分钟 K 线，返回时间升序。
	GetKline15MinuteAll(code string) (*KlineResp, error)
	// GetKline15MinuteUntil 读取十五分钟 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKline15MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKline30Minute 获取三十分钟 K 线的一页数据，分页参数同 GetKline。
	GetKline30Minute(code string, start, count uint16) (*KlineResp, error)
	// GetKline30MinuteAll 分页读取全部可用三十分钟 K 线，返回时间升序。
	GetKline30MinuteAll(code string) (*KlineResp, error)
	// GetKline30MinuteUntil 读取三十分钟 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKline30MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKline5Minute 获取五分钟 K 线的一页数据，分页参数同 GetKline。
	GetKline5Minute(code string, start, count uint16) (*KlineResp, error)
	// GetKline5MinuteAll 分页读取全部可用五分钟 K 线，返回时间升序。
	GetKline5MinuteAll(code string) (*KlineResp, error)
	// GetKline5MinuteUntil 读取五分钟 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKline5MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKline60Minute 获取六十分钟 K 线的一页数据，分页参数同 GetKline。
	GetKline60Minute(code string, start, count uint16) (*KlineResp, error)
	// GetKline60MinuteAll 分页读取全部可用六十分钟 K 线，返回时间升序。
	GetKline60MinuteAll(code string) (*KlineResp, error)
	// GetKline60MinuteUntil 读取六十分钟 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKline60MinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineAll 分页读取指定周期的全部可用 K 线，返回时间升序；偏移超过协议上限时报错。
	GetKlineAll(category uint8, code string) (*KlineResp, error)
	// GetKlineDay 获取日 K 线的一页数据，分页参数同 GetKline。
	GetKlineDay(code string, start, count uint16) (*KlineResp, error)
	// GetKlineDayAll 分页读取全部可用日 K 线，返回时间升序。
	GetKlineDayAll(code string) (*KlineResp, error)
	// GetKlineDayUntil 读取日 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKlineDayUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineMinute 获取一分钟 K 线的一页数据，分页参数同 GetKline。
	GetKlineMinute(code string, start, count uint16) (*KlineResp, error)
	// GetKlineMinute241Until 仅用于标准股票/ETF，拆分集合竞价生成 09:30 记录；停止条件同 GetKlineUntil。
	GetKlineMinute241Until(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineMinuteAll 分页读取全部可用一分钟 K 线，返回时间升序。
	GetKlineMinuteAll(code string) (*KlineResp, error)
	// GetKlineMinuteUntil 读取一分钟 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKlineMinuteUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineMonth 获取月 K 线的一页数据，分页参数同 GetKline。
	GetKlineMonth(code string, start, count uint16) (*KlineResp, error)
	// GetKlineMonthAll 分页读取全部可用月 K 线，返回时间升序。
	GetKlineMonthAll(code string) (*KlineResp, error)
	// GetKlineMonthUntil 读取月 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKlineMonthUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineQuarter 获取季 K 线的一页数据，分页参数同 GetKline。
	GetKlineQuarter(code string, start, count uint16) (*KlineResp, error)
	// GetKlineQuarterAll 分页读取全部可用季 K 线，返回时间升序。
	GetKlineQuarterAll(code string) (*KlineResp, error)
	// GetKlineQuarterUntil 读取季 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKlineQuarterUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineRange 获取包含两端日期的一分钟线；日期为 YYYYMMDD，扩展区间原始字段在 RangeExtended。
	GetKlineRange(code, startDate, endDate string) (*KlineResp, error)
	// GetKlineUntil 由近到远分页，stop 必须非 nil；返回时间升序并包含首次命中停止条件的记录。
	GetKlineUntil(category uint8, code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineWeek 获取周 K 线的一页数据，分页参数同 GetKline。
	GetKlineWeek(code string, start, count uint16) (*KlineResp, error)
	// GetKlineWeekAll 分页读取全部可用周 K 线，返回时间升序。
	GetKlineWeekAll(code string) (*KlineResp, error)
	// GetKlineWeekUntil 读取周 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKlineWeekUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetKlineYear 获取年 K 线的一页数据，分页参数同 GetKline。
	GetKlineYear(code string, start, count uint16) (*KlineResp, error)
	// GetKlineYearAll 分页读取全部可用年 K 线，返回时间升序。
	GetKlineYearAll(code string) (*KlineResp, error)
	// GetKlineYearUntil 读取年 K 线至 stop 命中，包含命中记录，语义同 GetKlineUntil。
	GetKlineYearUntil(code string, stop func(Kline) bool) (*KlineResp, error)
	// GetMarkets 获取市场表；MarketClient 合并标准内置市场和扩展发现结果，Conn 仅返回对应协议市场。
	// 发现市场不代表该市场已支持统一行情查询，扩展发现失败时返回错误。
	GetMarkets() ([]Exchange, error)
	// GetMinute 获取当日分时数据；时间保留源数据，不推断夜盘所属日期。
	GetMinute(code string) (*MinuteResp, error)
	// GetQuote 获取五档报价，保持输入顺序及重复代码；任一请求失败不返回部分结果。
	GetQuote(codes ...string) ([]Quote, error)
	// GetQuoteList 获取扩展市场批量行情列表；category 2 为港股、3 为期货，保留原始字段及单位。
	GetQuoteList(market, category uint8, start, count uint16) ([]protocol.QuoteListItem, error)
	// GetReportFile 通过标准报表协议下载指定文件的原始字节。
	GetReportFile(file string) ([]byte, error)
	// GetSpBlock 从标准 zhb.zip 获取专业板块成分，包含部分大型指数成分。
	GetSpBlock() ([]*protocol.SpBlock, error)
	// GetStockCodeAll 获取沪深北股票代码，返回值带市场前缀。
	GetStockCodeAll() ([]string, error)
	// GetTdxBk 从标准 zhb.zip 解析 tdxbk.cfg，获取概念板块简称与全称。
	GetTdxBk() ([]*protocol.TdxBk, error)
	// GetTdxHy 解析标准 tdxhy.cfg，获取股票的通达信及申万行业归属。
	GetTdxHy() ([]*protocol.TdxHy, error)
	// GetTdxStat 从标准 zhb.zip 解析 tdxstat.cfg，获取个股综合统计指标。
	GetTdxStat() ([]*protocol.TdxStat, error)
	// GetTdxStat2 从标准 zhb.zip 解析 tdxstat2.cfg，获取资金流向及板块归属。
	GetTdxStat2() ([]*protocol.TdxStat2, error)
	// GetTdxZs 从标准 zhb.zip 解析 tdxzs.cfg，获取板块名称与指数代码的对应关系。
	GetTdxZs() ([]*protocol.TdxZs, error)
	// GetTrade 获取当日一页分笔；start 从最新端偏移，count 为 1..1800。
	// 扩展分笔价格尺度未确认，公共 Price 为 nil，原始价格保留在 Extended.Price。
	GetTrade(code string, start, count uint16) (*TradeResp, error)
	// GetTradeAll 分页读取当日全部可用分笔，按旧页在前的顺序合并；中途失败不返回部分结果。
	GetTradeAll(code string) (*TradeResp, error)
	// GetXgsg 从标准 zhb.zip 解析 xgsg.cfg，获取新股申购列表。
	GetXgsg() ([]*protocol.TdxXgsg, error)
	// GetZHBFiles 下载并解压标准 zhb.zip，返回文件名到原始字节的映射。
	GetZHBFiles() (map[string][]byte, error)
	// SetTimeout 设置单次请求超时，覆盖排队、发送和等待响应；MarketClient 同步应用到现有及后续连接。
	SetTimeout(t time.Duration)
}

var _ Client = (*Conn)(nil)
var _ Client = (*MarketClient)(nil)
var _ protocolClient = (*StandardClient)(nil)
var _ protocolClient = (*ExtendedClient)(nil)

// 保留旧指针参数传 nil 的行为，避免接口包装空指针后被当成有效客户端。
func clientOrNil(c Client) Client {
	if c != nil && reflect.ValueOf(c).Kind() == reflect.Pointer && reflect.ValueOf(c).IsNil() {
		return nil
	}
	return c
}
