# v2 迁移指南

模块路径为 `github.com/injoyai/tdx/v2`，根包名仍为 `tdx`。下面的旧入口已移除，不再提供兼容别名。

## Client 接口与 Conn 连接实现

`tdx.Client` 现在是统一业务接口。`tdx.NewClient()` 返回该接口，默认实现为跨市场 `*MarketClient`；原单连接结构体改名 `Conn`，`Dial*` 返回 `*Conn`。两种实现均满足 Client，业务参数使用 `tdx.Client`，不使用 `*tdx.Client`。

| 旧调用 | 新调用 |
|---|---|
| 业务查询参数 `*tdx.Client` | `tdx.Client` |
| 单连接变量、底层字段或原始协议操作的 `*tdx.Client` | `*tdx.Conn` |
| 连接池工厂 `func() (*tdx.Client, error)` | `func() (*tdx.Conn, error)` |
| `IPool.Do/Go` 回调 `func(*tdx.Client)` | `func(*tdx.Conn)`，连接池继续管理单条连接 |
| `WithCodesClient/WithGbbqClient/WithWorkdayClient` | 接受 `tdx.Client`，支持统一门面和单连接 |
| `DialClientFunc`、上述三个组件的 DialClient 选项 | `func() (tdx.Client, error)`；提供连接工厂时用闭包适配返回类型 |
| `DialCodesFunc/DialGbbqFunc/DialWorkdayFunc` | 回调参数为 `tdx.Client` |
| 原 `Client.Info/Logger/Wait/SendFrame` 等底层能力 | 从 `*Conn` 使用；业务接口不暴露这些字段或原始发送方法 |

```go
var c tdx.Client = tdx.NewClient(tdx.WithRedial(false))
defer c.Close()
quotes, err := c.GetQuote("sz000001", "hk00700")
// 财务/F10/除权等标准能力也可通过同一对象调用，内部使用标准连接。
_ = quotes; _ = err

// 连接池仍按单连接管理。
pool, err := tdx.NewPool(func() (*tdx.Conn, error) { return tdx.DialDefault() }, 1)
if err != nil { panic(err) }
defer pool.Close()
```

`Client.Close()` 统一结束整个生命周期：门面关闭已有及晚到的连接，单连接取消运行并停止重连；可重复调用。`Conn.CloseAll()` 保留并使用同一关闭实现。原只关闭当次连接的 `Close` 行为有意改变，内部网络故障的重连路径保持原逻辑。

财务、F10、集合竞价、除权、板块、报表和标准分类代码列表已接入门面。它们保留原标准响应类型和单位，不因接口化获得港美股或期货数据支持；扩展品种、扩展单连接及非法标准代码会明确报错。无品种参数的标准专用查询由门面自动取得标准连接。

标准与扩展业务分别由 `StandardClient`、`ExtendedClient` 实现，两者均满足 `Client`；`MarketClient` 按市场路由到对应实现。`NewClient()`、`Dial*` 及连接池签名保持不变。需要具体实现时，使用 `NewStandardClient(conn)` 或 `NewExtendedClient(conn)` 包装已有对应协议连接；构造函数返回 `(具体客户端, error)`，拒绝空连接和错误协议。同一连接复用同一个业务实现及生命周期，关闭包装对象会关闭原连接，详见 [协议实现](unified-client.md#协议实现)。

## 跨市场统一查询

统一返回对象去掉 `Market` 前缀，名称对应如下；`Client`、`Conn`、`MarketClient` 保持现有名称。

| 旧返回类型 | 新返回类型 |
|---|---|
| `tdx.MarketQuote` | `tdx.Quote` |
| `tdx.MarketKlineResp` / `tdx.MarketKline` | `tdx.KlineResp` / `tdx.Kline` |
| `tdx.MarketMinuteResp` / `tdx.MarketMinute` | `tdx.MinuteResp` / `tdx.Minute` |
| `tdx.MarketTradeResp` / `tdx.MarketTrade` | `tdx.TradeResp` / `tdx.Trade` |
| `tdx.MarketCodeResp` / `tdx.MarketCode` | `tdx.CodeResp` / `tdx.Code` |
| `tdx.MarketExchange` | `tdx.Exchange` |
| `protocol.ExQuoteListItem` | `protocol.QuoteListItem` |

上述类型不保留旧名别名，外部类型声明和回调参数需同步改名；字段、单位和 JSON 形状保持不变。
`GetQuoteList` 仍只支持扩展市场；记录的 `Extended/RangeExtended` 原始协议类型保留 `Ex*` 名称，以区分不同的协议结构。

`tdx.NewClient()` 返回业务接口 `tdx.Client`，默认跨市场实现为 `*tdx.MarketClient`，按代码自动选择标准/扩展连接，并通过统一的 `GetQuote/GetKline*/GetMinute/GetTrade/GetCode/GetCodeAll/GetCount/GetMarkets` 返回统一结果。构造不联网，错误由首次查询返回。`Dial*` 和连接池仍可使用；单连接 `Conn` 的基础行情与代码查询已变更返回类型，须按下表迁移，HTTP 原路由与响应形状保持兼容；新结果不是旧 `protocol.Klines`，不能直接用于原指标/复权/存储 API。字段、单位及支持边界见 [统一 Client](unified-client.md)。

## 标准与扩展函数收敛（需迁移调用方）

不再公开同一业务的两套函数；以下移除入口没有兼容别名：

| 已移除入口 | 统一调用 |
|---|---|
| `ExQuote(market, code)` | `GetQuote("hk00700")`，返回列表，单品种取 `[0]` |
| `ExBars(category, market, code, start, count)` | `GetKline(category, "hk00700", start, count)`，记录位于 `List` |
| `ExMinute(market, code)` | `GetMinute("hk00700")` |
| `ExHistMinute(market, code, date)` | `GetHistoryMinute("20261009", "hk00700")` |
| `ExTrade(market, code, start, count)` | `GetTrade("hk00700", start, count)` |
| `ExHistTrade(market, code, date, start, count)` | `GetHistoryTrade("20261009", "hk00700", start, count)` |
| `ExQuoteList(...)` | `GetQuoteList(...)`，市场分页语义和参数不变 |
| `ExBarsRange(market, code, date, date2)` | `GetKlineRange("hk00700", "20261009", "20261009")`，一分钟区间，原扩展字段在 `RangeExtended` |
| `GetIndex/GetIndexUntil/GetIndexAll` 及指数周期便捷方法 | 对应 `GetKline/GetKlineUntil/GetKlineAll/GetKlineDay…`，自动选择指数解码；`GetIndexCodeAll` 是代码列表入口，仍保留 |
| `ExMarkets()` | `GetMarkets()`，返回 `[]Exchange`，原分类字段在 `Extended.Category` |
| `ExCount()` | `GetCount(exchange)`，返回指定市场的 `int` 数量，语义不再是全局总数 |
| `ExInstruments(start, count)` | `GetCode(exchange, start, count)`，按市场内偏移分页，返回 `*CodeResp` |

原标准 `Client.GetQuote/GetKline/GetMinute/GetHistoryMinute/GetTrade/GetHistoryTrade` 的返回类型也分别改为 `[]Quote`、`*KlineResp`、`*MinuteResp`、`*TradeResp`，与 `MarketClient` 相同。日期统一使用有效 `YYYYMMDD` 字符串；代码包含市场前缀，可用 `protocol.Instrument{Exchange: ex, Code: code}.String()` 构造。

统一价格为原币种/报价单位浮点值；未知量单位、扩展分笔价格和金额语义保持显式未确认。需要旧字段或旧数值单位时，从记录的 `Standard/Extended` 获取原始解码结构。例如旧 `quote.Kline.Close.Float64()` 改为统一 `quote.Price`，期货持仓从 `quote.Extended.ChiCang` 读取。标准成交转指标时，将每条 `Trade.Standard` 收集为 `protocol.Trades`；不要把未知扩展量按百股直接换算。

`DialDefault/DialExHqDefault` 仍只连接对应服务，通用 `Get*` 查询其他协议品种会报错；跨市场使用 `NewClient()`。`Client` 与 `MarketClient` 的全部 K线周期、`All/Until` 便捷方法均返回 `KlineResp`；`Until` 的回调参数改为 `Kline`。独立指数 K线查询和 `ExBarsRange` 均已移除，无兼容别名。统一代码路由仍只覆盖已接入的市场；此前直接用任意市场编号调用六组 `Ex*` 的代码，在未接入市场上需使用已有 `SendFrame` 与 `protocol.MEx.Frame*`，不能仅把市场编号替换成未知前缀。

完整成交查询 `GetTradeAll/GetHistoryTradeDay/GetHistoryTradeBefore/GetHistoryTradeFull` 统一返回 `*TradeResp`，支持两类协议。单日历史响应和逐条 `Trade.Date` 保留请求日期；跨日响应的整体 `Date` 为空，每条记录有自己的日期。分页顺序、协议偏移 65535 护栏及标准跨页昨收保持；中途失败不返回部分历史。`Count` 使用 int，原标准 uint16 结果需要显式转换。

历史分笔查询新增同代码、同交易日期的日线检查：没有对应日线时返回空分笔，日线查询失败时返回错误；整日分页仅检查一次，跨日查询逐日检查。调用方不能再依赖无对应日线时分笔接口的非空返回；当天实时分笔不受影响。此保护可能增加日线分页请求，详见 [统一 Client](unified-client.md)。

`GetHistoryTradeBefore/Full` 的交易日历参数改为 `TradingCalendar`（提供 `Range` 方法）。现有 `*Workday` 可继续传入，但它是沪深交易日历；其他市场必须由调用方提供对应日历。查询从最早可获取月线所对应月份开始，不保证服务器保留上市以来的全部分笔；`Before` 沿用日历的日期结束边界，获取整日数据，不按结束时刻截断当天分笔。扩展夜盘日期和价格尺度不作猜测换算。

标准指标、复权、存储和旧 HTTP 结构可使用 `resp.StandardResponse()` 获取原 `protocol.KlineResp/TradeResp`。它只提取原始字段，不从浮点价格重建整数，不将扩展字段冒充标准数据；扩展结果和超过 uint16 数量上限的结果会报错。例如：

```go
r, err := c.GetKlineDayAll("sz000001")
if err != nil { return err }
raw, err := r.StandardResponse()
if err != nil { return err }
// raw.List 可继续用于 protocol.Klines 的指标或标准复权 API。
```

`GetKlineRange(code,startDate,endDate)` 固定查询一分钟线，日期使用 YYYYMMDD，包含两端日期。扩展保留原区间协议的固定请求周期 7（与普通扩展一分钟查询的周期映射不同），持仓和结算价等字段在 `RangeExtended`，金额及量单位未确认时保持未知；标准通过分页筛选日期区间，超出可读取分页范围时报错。未接入的协议市场仍须使用 `SendFrame`。`GetKlineMinute241Until` 在两种 Client 上使用同一签名及统一返回类型，仅适用于标准股票/ETF；指数或扩展品种明确报错，普通分钟查询使用 `GetKlineMinuteUntil`。

代码查询在 `Client` 与 `MarketClient` 上使用相同签名：`GetCount(exchange) (int, error)`、`GetCode(exchange, start uint32, count uint16) (*CodeResp, error)`、`GetCodeAll(exchange) (*CodeResp, error)`。原标准 `GetCode(exchange, start)` 需增加 `count`（旧分页大小为 1000）；`code.Code` 改为 `code.Instrument.Code`，小数位等特有元数据从 `code.Standard` 读取。分页 `count` 为 1..1000；扩展协议需扫描全局列表再按市场过滤，偏移按过滤后的列表计算。北交所数量和分页使用与完整列表相同的报表来源。

`Conn.GetMarkets()` 在标准连接上返回内置沪深北，在扩展连接上返回服务器市场表；`NewClient().GetMarkets()` 合并两者，并在扩展发现失败时返回错误。服务器列出的市场不一定已接入统一行情查询。需要旧扩展全局总数或全局分页时，使用既有 `SendFrame(protocol.MEx.FrameCount())` / `SendFrame(protocol.MEx.FrameInstrument(start, count))`，不能用任意单市场查询替代。历史 HTTP `/ex/count`、`/ex/instruments` 继续通过协议层保持原全局语义。pull 的全局品种快照入口改名 `Service.GetInstruments()`，本轮缓存范围保持不变。

HTTP 内部已迁移到统一函数，但仍返回原 `protocol` 数据，原价格单位、字段名和空列表形状不变。扩展日线/一分钟参数经市场周期映射；特殊市场、周期能否取得数据仍以实际服务支持为准。内置 pull 接口已同步迁移并保留原存储口径。

## 接口替换

| 已移除的入口 | v2 入口 |
|---|---|
| `protocol.HistoryTradeResp` | `protocol.TradeResp` |
| `Client.GetMinuteTrade` | `Client.GetTrade` |
| `Client.GetMinuteTradeAll` | `Client.GetTradeAll` |
| `Client.GetHistoryMinuteTrade` | `Client.GetHistoryTrade` |
| `Client.GetHistoryMinuteTradeDay` | `Client.GetHistoryTradeDay` |
| `Client.GetKlineHour` | `Client.GetKline60Minute` |
| `Client.GetKlineHourAll` | `Client.GetKline60MinuteAll` |
| `Client.GetKlineHourUntil` | `Client.GetKline60MinuteUntil` |
| `extend.NewPullKline` / `PullKlineConfig` / `PullKline` | `pull.NewService` / `pull.Config` / `pull.Service` |
| `extend.GetBjCodes` / `lib/bse.GetCodes` | `Client.GetCodeAll(protocol.ExchangeBJ)` |

另外移除或统一以下入口：

| 已移除的入口 | v2 用法 |
|---|---|
| `protocol.TypeMinuteTrade` / `TypeHistoryMinuteTrade` | `TypeTrade` / `TypeHistoryTrade`，消息数值不变 |
| `protocol.Factor.QFQ` / `HFQ` | `QFQPrice(raw)` / `HFQPrice(raw)`，或使用 `Mul * raw.Float64() + Add` |
| `PreKline.PreLast` / `Factor.PreLast` / `QFQFactor()` / `HFQFactor()` | `XRXDs.Pre(ks).Factors()` 获取仿射系数；单事件参考价用 `XRXD.Pre(raw)` |
| `extend.Kline` / `extend.Klines` | 指标使用 `protocol.Kline/Klines`；落库模型使用 `pull.KlineDay/KlineMinute`，注意单位转换 |
| `extend.Klines.REF/HHV/LLV` | 调用方检查长度后按切片取值或遍历计算高低值 |
| `extend.Info` | 调用方按需要组合行情、股本和换手率数据 |
| `extend.DefaultDayKlineExportTitle` / `DefaultMinuteKlineExportTitle` | CSV 导出使用 `KlinesToCsv`；自定义列由调用方定义 |

`protocol.Klines` 成为唯一指标实现。VWAP 的输入成交量须为手，成交额须为厘；`pull` 查询结果使用股和元，需要先转换。标准行情指数日线的解码成交量为股，调用 VWAP 前也需转为手。VWAP 修正手股换算，ATR 使用最近 n 个真实波幅，RSI 在纯上涨窗口返回 100、平盘窗口返回 0。RSI/ATR 为已有的简单滚动窗口口径，不改为其他平滑算法。

第三方爬虫使用的 `extend.Factor/Factors` 继续保留，它们是数据源返回的比例模型，不等同于通达信仿射复权模型。

## 请求等待与超时

`SendFrame` 先注册回调再发送，避免快速响应早于等待注册而被丢弃；回调注册与响应处理互斥，超时或写入失败会清理回调与请求缓存，迟到响应会被忽略。标准行情默认请求超时仍为 2 秒，扩展行情仍为 10 秒，覆盖排队、发送和等待响应全过程。发送中途超时或短写会关闭当次连接，避免残缺帧污染后续请求；仅排队或等待响应超时则保留连接。

标准与扩展行情的握手、心跳、业务请求和重连初始化共用写入门控，保护底层连接状态与写入统计；`Client.Write` 及其字符串等写入方式保持稳定入口。心跳绑定当次连接。`CloseAll()` 取消连接生命周期、关闭传输并等待读取流程退出后释放底层资源，不再并发修改底层重连开关。截断或非法的成交记录、可关联请求的帧解压错误直接传回调用方，不再表现为等待超时。

扩展行情响应的头部或声明的记录不足时返回 `io.ErrUnexpectedEOF`，不再返回部分记录或把损坏响应当成空结果；合法的零记录响应仍正常返回。HTTP 服务的 `Close()` 同时释放自建的标准与扩展行情连接池，初始化失败也会清理已建连接池。

TCP 传输使用幂等关闭封装，避免重复关闭导致底层库跳过断开回调与缓存清理；`Origin()` 返回值仍支持 `net.Conn` 接口，调用方应依赖接口而非断言具体的 `*net.TCPConn` 类型。

真实只读验证按 `PROJECT_RULES.md` 第 8.5 节执行。标准与扩展行情测试共用每秒一次、最多 1000 次的写入限流，可用 `TDX_STD_LIVE=1 TDX_EXHQ_LIVE=1 go test -race . -run '^(TestStandardLive|TestExHqLive)$' -count=1 -v -timeout=150s` 启用；分别用 `TDX_STD_HOST` / `TDX_EX_HOST` 指定不含端口的服务器 IP。标准测试只做一次受控重连，普通离线测试跳过真实访问。HTTP 上游验证用 `TDX_HTTP_LIVE=1 go test -race ./extend/httpserver -run '^TestServerLive$' -count=1 -v -timeout=45s`，须在行情测试完成后单独执行，不并行启动。

数据准确性还须按项目规则第 8.6 节对照独立来源，其他来源单线程、请求起始间隔至少 5 秒。`TDX_CROSS_LIVE=1 go test -race . -run '^TestCrossValidationSamplesLive$' -count=1 -v -timeout=45s` 保存小样本到 `output/live-validation/cross/tdx-samples.json`，供另行串行获取独立数据并逐字段比较；该测试成功只表示样本获取成功。港股主板日线现用 category=4，腾讯控股样本的 `ExKline.Trade` 为百股，×100 与独立来源在整数百股精度内一致；不能将该单位推广到其他扩展接口。小时线的来源聚合差异和港股分时的时间标签仍须确认。

标准行情代码、K 线与五档报价也检查记录边界和变长整数；截断或溢出响应直接返回解码错误。`protocol.MQuote.Decode` 保留原返回类型，异常数据返回 `nil`；需要错误详情时改用 `protocol.MQuote.DecodeChecked`。

`NewHostDial`、`NewRandomDial`、`NewRangeDial`、`NewExRangeDial` 使用调用方的 context 建连，每个地址最多等待 5 秒；遍历地址间的退避等待也可取消。`NewManage` 初始化失败会关闭内部创建的、支持 `Close()` 的连接池；`WithPool` 传入的外部池仍由调用方关闭。旧 `extend.ListenCodesHTTP` / `ListenCodesAndGbbqHTTP` 初始化失败会返回错误。

代码元数据刷新使用 SQLite 增量事务，保留未变化记录和原主键，更新名称、倍数、小数位与昨收价，删除退市项；内存缓存使用提交后的数据库快照。

设置请求超时统一使用 `Client.SetTimeout(duration)`；原先直接调用单连接 `Client.Wait.SetTimeout` 的代码需迁移到该方法。底层 `Conn.Wait` 保留原类型，用于内部响应关联，直接修改它不会调整 `SendFrame` 的计时器。

## 成交行为

成交接口的参数、返回类型、分页顺序和请求协议保持一致；HTTP 服务的成交路由与响应格式保持一致。

标准行情的全部代码、成交、K 线分页使用整数累加偏移；偏移或累计数量超过 `uint16` 上限（65535）会报错，不会回绕或返回错误数量。此时调用方不应将结果视为完整历史。`GetBlockFileRaw` 按元信息校验文件长度，缺块返回 `io.ErrUnexpectedEOF`，过长分块也会报错。

成交示例统一见 [GetTrade](../example/GetTrade/main.go)。

北交所代码通过通达信 `zhb.zip` 中的 `tdxbjmore.cfg` 获取，返回 `CodeResp.List`，包含代码与名称。需要 `bj` 前缀字符串时，将 `"bj"` 拼接到每条记录的 `Code`；该配置可能包含已分配但尚未上市的代码，不等同于官网上市证券列表。示例见 [GetBjCodes](../example/GetBjCodes/main.go)。

## K 线拉取服务

统一使用 `github.com/injoyai/tdx/v2/extend/pull`。使用内置市场时需要注册适配器：

```go
import (
    "github.com/injoyai/tdx/v2/extend/pull"
    _ "github.com/injoyai/tdx/v2/extend/pull/market"
)
```

| 旧调用 | 新调用 |
|---|---|
| `Types: []string{extend.Day, extend.Minute}` | `Day: true, Minute: true` |
| `Types: []string{extend.Day}` | `Day: true, Minute: false` |
| `Update(m)` | 在 `Config.Manage` 中传入 `m`，然后调用 `s.Update()` |
| `Run(m, spec)` | 调用方安排定时任务，在任务内调用 `s.Update()` |
| `DayKlines(code, start, end)` | `s.QueryDay(c, start, end)`，其中 `c` 由 `pull.ParseCode(code)` 获取 |
| `DayKlinesAll(code)` | `s.QueryDay(c, time.Time{}, time.Time{})` |
| `DayKline(code, n)` | 查询升序日线后检查长度，按切片下标选取；最新一根为 `ks[len(ks)-1]` |
| `MinKlines(code, start, end)` | `s.QueryMin(c, start, end)` |

`Dir` 必须显式设置；`StartAt` 改用 `YYYYMMDD` 字符串，未设置时默认最近两年，非法格式或不存在的日期在创建服务时直接报错，且不会创建数据库。`Day` 与 `Minute` 都为 `false` 时默认同时拉取两种数据。服务使用完成后调用 `s.Close()`，外部连接池仍由调用方关闭。

日线查询返回 `[]*pull.KlineDay`，分钟查询返回 `[]*pull.KlineMinute`；价格与金额为元，已确认单位的股票/ETF/指数/板块及港股日线成交量为股。将价格交给使用厘单位的 `protocol` API 时需转换单位。示例见 [PullKline](../example/PullKline/main.go) 和 [Income](../example/Income/main.go)，完整服务说明见 [pull 文档](../extend/pull/README.md)。

内置市场的 K 线参数和成交量换算现集中于 `Market.Info()`；由 `ParseCode` 得到市场后可直接查询。未知单位设为 `VolumeDefault`，暂按旧倍率 ×100，不能据此认定这些扩展市场的量单位为股或合约。配置只覆盖日线和 1 分钟线。

港股主板日线由 category=9 改为 4，修正成交量请求口径；标准行情指数及板块分钟线直接使用解码后的股数，取消拉取层重复 ×100。已有数据库不会自动修正历史记录；旧港股日线首条的异常量不能靠整体乘倍数恢复，重新获取时请保留旧库，并考虑服务器历史保留范围。

## 数据库支持

内置数据库支持统一使用 SQLite，已移除 MySQL 驱动注册、直接依赖和专用示例。以下入口不再提供：

| 已移除的入口 | SQLite 用法 |
|---|---|
| `tdx.NewManageMysql` | `tdx.NewManage` |
| `tdx.NewCodesMysql` | `tdx.NewCodes` / `tdx.NewCodesSqlite` |
| `tdx.NewWorkdayMysql` | `tdx.NewWorkday` / `tdx.NewWorkdaySqlite` |
| `xorms.NewMysql` | `xorms.NewSqlite(filename)` |

通过 `WithCodesDB`、`WithWorkdayDB`、`WithGbbqDB` 或对应 `DialDB` 选项传入 SQLite 引擎，可自定义数据库路径。已有 MySQL 数据不会自动转换为 SQLite；需要保留数据时另行导出转换，不能直接将 MySQL DSN 当作 SQLite 文件名。

## 存量数据库

旧服务使用 `day-kline/`、`min-kline/` 与 `update.db`；新服务按市场存储，例如 `cn/stock/day/sz000001.db`、`cn/stock/min/sz000001/sz000001-2026.db`，去重库为 `updated.db`。

旧表与新表的字段结构、时间字段和量价单位不同，不能仅重命名目录直接复用。新服务不会自动读取或转换旧服务数据库；需要单独转换存量数据，或在新目录重新拉取。重新拉取只能恢复服务器仍可提供的历史，旧库应保留以便转换。

新服务自身已有 SQLite 表的补字段、覆盖标记兼容继续保留，与上述旧服务目录迁移是两回事。

## 默认缓存目录

Go 最低版本为 1.25.0。默认数据目录从 `./data` 调整为 `./output/data`，代码、交易日和股本缓存使用 `./output/data/database`；分笔 CSV 示例输出到 `./output/trade`。

旧缓存不会自动搬迁。需要保留现有代码/交易日/股本缓存时，关闭所有使用它们的进程后，将 `codes.db`、`workday.db`、`gbbq.db` 及对应 SQLite WAL/SHM 文件完整复制到新目录，保留原文件备份；这些缓存表结构不变。也可以通过 `WithCodesDB/WithWorkdayDB/WithGbbqDB` 或对应 `DialDB` 选项显式配置路径。这类元数据缓存迁移与上面的旧 K 线数据库格式转换不同。
