package pull

// Market 市场标识枚举（Unit.Name() / Code.Market / Config.Codes 路由结果）。
//
// 定义在 pull 包（而非 market 子包）是因为注册表与 Code 模型都在本包，
// market 子包注册时直接引用常量，避免魔法字符串散落。
type Market string

// 内置市场。注意与 protocol.Exchange（uint8 协议层市场编码）是两套体系：
// 这里的 Market 是"拉取市场类别"（一个 Unit 一类，如期货覆盖多个交易所），
// 值同时用作存储目录名、日志前缀与增量去重键的一部分，勿随意改动。
const (
	// 枚举值即存储目录路径（Code.DirName 直接返回枚举值），
	// 两级「地区/资产」形式，与磁盘布局完全一致。
	MarketAStock  Market = "cn/stock"  // 沪深股票
	MarketIndex   Market = "cn/index"  // 沪深指数
	MarketEtfLof  Market = "cn/etf"    // ETF/LOF
	MarketBlock   Market = "cn/block"  // 板块指数
	MarketFuture  Market = "cn/future" // 期货（中金/郑商/大商/上期/广期等，走扩展行情）
	MarketHK      Market = "hk/stock"  // 港股主板（走扩展行情）
	MarketHKIndex Market = "hk/index"  // 港股指数（恒生系/中华系，扩展行情市场27）
	MarketUS      Market = "us/stock"  // 美股（股票/ETF/指数混合，走扩展行情；协议层无法区分）
)

// String 实现 fmt.Stringer。
func (m Market) String() string { return string(m) }
