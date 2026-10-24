# 统一行情 Client

`tdx.NewClient()` 返回统一业务接口 `tdx.Client`，默认实现为 `*tdx.MarketClient`。一个业务对象按证券代码自动选择标准行情或扩展行情，返回同一组统一结果类型。标准与拓展连接分别由 `*tdx.StConn`、`*tdx.ExConn` 实现业务接口；原 `Conn` 已移除，连接池使用 `Client`；单连接的基础行情、全部 K线便捷方法及完整分笔查询统一为同名 `Get*` 和统一返回类型。公开 `ExQuote/ExBars/ExMinute/ExHistMinute/ExTrade/ExHistTrade` 已移除，`ExQuoteList` 改名为 `GetQuoteList`；代码列表与数量使用统一 `GetCode/GetCodeAll/GetCount`，`ExMarkets` 改为 `GetMarkets`，`ExBarsRange` 收敛为 `GetKlineRange`，指数 K线查询并入 `GetKline*`；HTTP 查询直接返回同一套统一模型，见 [HTTP 服务](../extend/httpserver/README.md)。

## 使用

```go
c := tdx.NewClient(tdx.WithRedial(false))
defer c.Close()
c.SetTimeout(10 * time.Second)

a, err := c.GetKlineDay("sz000001", 0, 100)
if err != nil { return err }
h, err := c.GetKlineDay("hk00700", 0, 100)
if err != nil { return err }
f, err := c.GetKlineDay("cffIF2609", 0, 100)
if err != nil { return err }
_ = a; _ = h; _ = f

quotes, err := c.GetQuote("sz000001", "hk00700", "usAAPL")
if err != nil { return err }
for _, q := range quotes {
    fmt.Println(q.Instrument.String(), q.Price)
}
```

构造不联网，因此没有构造错误返回值；首次访问某类协议时建连，连接错误由查询方法返回。两类协议各复用一条连接，默认服务器和重连策略沿用 `DialDefault`/`DialExDefault`；`client.Option` 同时应用于两类连接。可并发查询；`SetTimeout` 覆盖当前和未来连接，默认仍为标准 2 秒、扩展 10 秒。`Close` 先取消首次建连、退避重试及后续连接生命周期，释放已建立的连接，再等待建连结束并释放晚到的连接，允许重复/并发调用，关闭后不会重新建连。两类连接各自控制初始化；一类建连变慢不会阻塞另一类查询或 `SetTimeout`。每个 TCP 地址的建连超时为 5 秒，请求超时不控制建连重试时长；默认自动重试可以通过 `WithRedial(false)` 关闭，进行中的首次重试也可以通过门面的 `Close` 取消。必须通过构造函数使用，零值不用于查询。

ETF、可转债等标准报价需要价格精度元数据时，使用同一连接取得所属交易所代码表，仅缓存在该连接的 `StConn` 实现中，不依赖全局缓存、数据库或额外连接。已缓存代码复用精度，缺少代码时重新读取对应市场；缺失精度信息会返回错误，不猜测价格。

代码缓存服务位于独立的 [`extend/codes`](../extend/codes/README.md) 包，对外接口为 `codes.Interface`。标准连接的价格精度缓存与扩展代码服务共享内部纯内存实现，不会反向引用扩展包或触发数据库、定时任务初始化。

## 协议实现

源码按职责集中：`client_interface.go` 定义统一接口；`client.go` 集中 `MarketClient` 门面、市场路由、统一返回模型和标准结果转换，对应测试放在 `client_test.go`。`conn_st.go` 包含标准连接、建连入口、业务请求和解码；`conn_ex.go` 对应拓展连接。内部 `conn.go` 仅负责收发、请求关联和生命周期；`query.go` 及 `conn_query.go` 保存共享业务算法及校验。内部传输不实现 `Client`，业务查询不经传输层转发到门面。

根目录保留需要访问包内实现的测试：`client_test.go` 验证统一门面、接口和结果转换；`conn_test.go` 验证共享传输及连接生命周期；`conn_query_test.go` 验证标准/扩展业务查询、代码列表和报表文件；`query_test.go` 验证分页、停止条件及历史日期检查。`dial_test.go`、`hosts_test.go`、`pool_test.go` 分别覆盖拨号、服务器地址和连接池。两类协议共用的表驱动用例保持在同一个文件中，管道模拟和协议收发辅助函数集中在 `test_helpers_test.go`；`conn_live_test.go` 集中实连测试及限流辅助函数，仍须通过原有环境变量显式启用。

只依赖公开接口的跨包契约、元数据资源归属及数据一致性测试位于 [`tests/integration/`](../tests/integration/README.md)，使用 `go test ./tests/...` 运行，`go test .` 不包含这部分测试。管理器实现与自身回归测试位于独立的 `extend/manage` 包，构造使用 `manage.New()`，详见 [管理器说明](../extend/manage/README.md)。

`StConn`、`ExConn` 分别实现 `Client`，`MarketClient` 通过同一接口选择两者。混合报价仍保持原输入顺序，标准品种合并请求、拓展品种逐个请求。共享分页、停止条件和历史交易日期检查直接调用绑定业务对象的基本查询。

通常只需 `NewClient()`。需要单协议连接时直接建连：

```go
standard, err := tdx.DialDefault(tdx.WithRedial(false))
if err != nil { return err }
var c tdx.Client = standard
defer c.Close()
quotes, err := c.GetQuote("sz000001")
_ = quotes; _ = err
```

拓展连接使用 `DialExDefault()`。两种连接各自指定握手、心跳、默认超时和解码，共享内部传输实现；不再使用 `NewStConn(conn)` / `NewExConn(conn)` 包装，也不对外暴露通用 `Conn`。通过建连入口取得可用对象，零值不用于查询。

标准客户端不支持扩展证券和行情分页列表；扩展客户端不支持标准证券及财务、F10、除权、板块、报表等标准专用接口，均在发送前报错。实现同一接口不代表服务器的数据能力相同。

## 代码与市场

| 输入示例 | 协议市场 |
|---|---|
| `sz000001`、`sh.600000`、`600000.SH` | 标准沪深 |
| `bj920001`、`bj899050` | 标准北交所 |
| `hk00700`、`HK.00700`、`00700` | 扩展港股 31 |
| `hk.HSI`、`hkCES120`、`hi.HZ5021` | 扩展港股指数 27 |
| `usAAPL`、`us.BRK.B`、`SHOP` | 扩展美股 74 |
| `cffIF2609`、`cff.IF2609`、`cff/IF2609` | 扩展中金 47 |
| `czc`、`dce`、`shf`、`gfe`、`qhz` + 合约代码 | 扩展 28、29、30、66、42 |

前缀不区分大小写，港美股主体统一大写，期货主体保留大小写。`HSI/HKL/VHSI/CES100/GEM` 裸代码沿用 pull 的港股指数规则；其他有歧义的品种须显式带前缀，裸 `IF2609` 会报错。标准代码须为 6 位数字，扩展主体最多 9 个 ASCII 字节，超长或非法输入在发送前拒绝，避免底层定长字段截断。

`protocol.ParseInstrument` 返回 `{Exchange, Code}`，`Instrument.String()` 返回点分隔代码，港股指数统一为 `hk.HSI`。统一门面覆盖上表市场；期权、外汇、新三板、其他扩展市场和 `pull` 自定义市场不因存在市场枚举而自动获得支持。未接入统一路由的市场可使用既有 `SendFrame` 配合 `protocol.MEx.Frame*` 请求，并按响应类型取原始结构；不再有公开的 `Ex*` Client 行情业务方法。

业务变量和参数使用 `Client` 接口。`NewClient()` 按需连接两类协议，标准 `Dial*` 返回 `*StConn`，`DialEx*` 返回 `*ExConn`。`Close()` 结束生命周期并停止重连，`SetTimeout()` 配置请求超时。原始 `SendFrame`、连接状态和日志等从具体连接使用，业务接口不包含这些能力。连接池工厂为 `func() (Client, error)`，Get/Put/Do/Go 均使用 `Client`；原始扩展全局请求须取得池中 `*ExConn` 后发送，HTTP/pull 已同步适配。完整迁移见 [v2 指南](v2-migration.md)。

## 查询接口

| 方法 | 返回 |
|---|---|
| `GetQuote(codes...)` | `[]Quote` |
| `GetQuoteList(market, category, start, count)` | `[]protocol.QuoteListItem`，仅扩展市场 |
| `GetKline(category, code, start, count)` | `*KlineResp` |
| `GetKlineMinute/5Minute/15Minute/30Minute/60Minute/Day/Week/Month/Quarter/Year` | `*KlineResp` |
| `GetKlineAll/GetKlineUntil`，以及全部周期便捷方法的 `All/Until` | `*KlineResp` |
| `GetKlineRange(code, startDate, endDate)`、`GetKlineMinute241Until(code, stop)` | `*KlineResp` |
| `GetMinute(code)`、`GetHistoryMinute(date, code)` | `*MinuteResp` |
| `GetTrade(code, start, count)`、`GetHistoryTrade(date, code, start, count)` | `*TradeResp` |
| `GetTradeAll/GetHistoryTradeDay/GetHistoryTradeBefore/GetHistoryTradeFull` | `*TradeResp` |
| `GetCode(exchange, start, count)`、`GetCodeAll(exchange)` | `*CodeResp` |
| `GetCount(exchange)` | `int`，指定市场数量 |
| `GetMarkets()` | `[]Exchange`，市场发现结果 |

日期为有效 `YYYYMMDD`。K 线单次数量 1..800，实时/历史分笔单次数量分别为 1..1800/1..2000。K 线周期使用既有 `protocol.TypeKline*` 常量；`Day/Day2` 和 `Minute/Minute2` 在业务入口分别视作日线、一分钟别名，实际周期按市场配置映射，记录在返回的 `Category` 中。其他周期沿用协议编号，能否取得数据取决于服务器，不能把成功路由视为每个市场/周期已经通过实盘验证。

标准股票、指数和板块指数统一调用 `GetKline*`，内部自动选择股票/指数解码，不需要另选 `GetIndex*`。日线/一分钟的协议、分类与量单位配置下沉到 `protocol.Market.Info()`，pull 直接使用 `protocol` 的市场类型、常量和量单位换算。

报价先解析全部输入再发送，标准品种合并请求，扩展品种顺序请求；结果保留原输入顺序与重复项。任何连接、请求或响应品种校验失败返回错误，不返回部分报价；标准请求超过 `uint16` 帧长上限会提前报错。混合结果不保证来自同一瞬间。

K 线分页按从近到远查询，返回列表为旧到新，`Until` 包含首次命中的边界记录。偏移超过 65535 返回错误，不回绕、不把部分历史当完整结果。返回 `Count` 为 `int`，不沿用旧响应总数的 `uint16` 限制。

代码分页 `start` 为 `uint32`，表示指定市场内偏移，`count` 为 `uint16`，有效范围 1..1000。标准沪深协议每次最多 1000 项，按请求数量截取；沪深偏移超过 65535 提前报错。扩展品种按全局序号分页，再按 `Exchange` 过滤，分页偏移作用于过滤后的列表；统一 `GetCount` 返回指定市场数量，因此扩展数量和分页查询均会扫描全局列表，暂不引入跨查询缓存。超出列表的页返回空列表。北交所沿用 `zhb.zip/tdxbjmore.cfg`，可能包括已分配未上市代码。列表不自动推断美股的股票/ETF/指数类别。

`GetMarkets` 的 `Exchange` 包含协议市场编号 `Market`、名称、简称和 `Protocol`；扩展原始分类等字段保留在 `Extended`。标准单连接返回内置沪深北，扩展单连接返回服务器市场表；跨市场入口合并两者，只需建立扩展连接，发现失败不返回部分结果。发现市场不意味着对应行情路由已接入。旧扩展全局计数和全局列表分页可使用 `SendFrame` 与 `protocol.MEx.FrameCount/FrameInstrument`，历史 HTTP 接口及 pull 共用快照保持原语义。

`GetKlineRange` 固定查询一分钟区间，日期包含两端；扩展使用原固定周期 7 的区间协议（不映射为普通扩展分钟周期 8），结果原字段保留在 `RangeExtended`，量单位和金额仍保持未确认。标准通过分页筛选，超出偏移上限报错。服务器可能限制历史可用范围；请求成功不保证长期完整覆盖。`GetKlineMinute241Until` 保留标准股票/ETF 集合竞价拆分，其他品种报错，不套用 241 根的交易时段。

完整分笔按递增偏移查询，较早页放在结果前面；偏移超过 65535、响应超出请求数量或请求失败均报错。跨日 `Before/Full` 需要实现 `TradingCalendar.Range` 的对应市场日历，中国市场历史日历使用 `*workday.Workday`（`extend/workday`）；不自动用沪深日历判断港美股或期货交易日。每条历史分笔的 `Date` 为请求交易日期，夜盘自然日和未知价格尺度仍不推断。查询始于服务器最早可获取月线所在月份，不保证上市以来的全部历史。

历史分笔 `GetHistoryTrade/GetHistoryTradeDay` 在请求前查询同代码、同交易日期的日线；`Before/Full` 逐日沿用此检查。没有对应日线时不发送分笔请求，返回保留证券及日期的空响应（`Count=0`）；日线查询失败直接返回带代码、日期的错误。整日分页仅在首个分笔请求前检查一次，单页查询每次检查，包括非零偏移。检查只要求日期存在，不以日线成交量判定；当天实时 `GetTrade/GetTradeAll` 保持原行为。较早日期可能需要多页日线查询；服务器不再保留对应日线时，即使分笔接口非空也会跳过，日线存在也不保证分笔内容准确。

## 数据契约

- 统一价格为 `float64`，使用证券原币种/报价单位；标准整数厘通过现有 `Float64()` 转换，扩展浮点价格保留底层精度。没有汇率换算，也不把美元、港币或指数点标为人民币元。
- 成交量保留解码数值，每个接口独立标注 `VolumeUnit`：`hundred_shares` 为协议百股，`shares` 为股，`unknown` 为单位尚未确认。`unknown` 不乘 100。当前已配置的标准股票/ETF K 线为百股、标准指数/板块 K 线为股、港股日线为百股；可转债及未确认扩展周期为 unknown。报价、分时、分笔均保守标为 unknown，不套用 K 线单位配置。
- K 线 `Amount` 为指针，标准金额有值；扩展金额与持仓字段重叠，统一金额暂为 nil。期货持仓、结算价等保留在 `Extended` 中。原始结构的字段仍按各自既有契约解释，存在字段不代表其语义在全部市场已确认。
- 分笔公共 `Direction` 为 1 买、-1 卖、0 中性。扩展分笔的原始整数价格尺度未确认，公共 `Price` 为 nil，原始值保留在 `Extended.Price`；不会擅自除以 100/1000。[pytdx 上游分笔解析](https://github.com/rainx/pytdx/blob/master/pytdx/parser/ex_get_transaction_data.py) 同样保留整数原始价格。
- 每条数据的 `Standard` 或 `Extended` 保留完整的对应底层结构，避免丢失涨跌家数、均价、持仓、增仓、开平性质等特有信息。
- K 线 `Time` 保留源精度：日及更长周期为 `YYYY-MM-DD`，日内周期为 `YYYY-MM-DD HH:MM`；分时／分笔为 `HH:MM`／`HH:MM:SS`。不为日线补造 15:00。`Kline.TradingDate` 仅填写已确认交易日；扩展未知为空，历史分笔的 `Date` 仍是请求交易日期，不等同于夜盘自然日。
- `KlineResp`、`MinuteResp`、`TradeResp` 的 `TimeZone` 表示来源时区：标准行情为固定 `UTC+08:00`，扩展未知为空。不根据证券所在地猜测服务器时间口径，不修改全局 `time.Local`。`Kline.Timestamp(loc)` 仅在显式给出来源时区、且记录含时分时转换；未知时区及夏令时不存在／重复的钟面时刻报错。显示时区由调用方决定。
- 标准原始 `Kline.Time`／`Trade.Time`／`Gbbq.Time`、本地文件及同花顺适配沿用 `protocol.Location()` 的固定东八区约定；原始日线的 15:00／00:00 是内部日期标签。pull 按源日期时分保存和查询，扩展不再按东八区生成 Unix。新存储格式及接口变更见 [迁移说明](v2-migration.md#源时间交易日与真实时刻)。

`Dial*` 返回的单协议连接 与 `NewClient()` 门面共享基础行情与代码查询的名称、参数和返回结构。单连接绑定建连时的协议，查询另一类品种会返回协议不匹配错误，不会额外建连；混合市场查询使用 `NewClient()`。K线全部周期及 `All/Until`、完整分笔查询均返回统一类型；指数 K线自动选择解码，独立 `GetIndex*` K线入口已移除。`Until` 回调接收 `Kline`，标准跨页昨收保留在原始 `Standard.Last`。财务、除权、公司资料、集合竞价、板块及报表等标准能力已接入业务接口，门面自动选择标准连接；扩展品种和扩展单连接不支持这些接口。它们保持原标准返回类型与单位。统一结果不是 `protocol.Klines`，不能直接传给现有指标、复权或存储方法；需要原始协议响应时调用 `StandardResponse()`；它保留整数价格、时间和原始量单位，不转换扩展数据，并检查旧 uint16 数量上限。指标和本地文件编码使用下述 `Klines()` 整数手适配。K 线、分时和分笔都提供该方法；Count 按实际 List 长度生成，缺少 Standard 原始记录时返回错误。

## 指标与本地文件的量单位适配

`KlineResp.StandardResponse()` 保留原始记录及其单位，适用于需要协议原始字段的复权等处理；标准指数的原始成交量仍为股。`VWAP()` 与 `.day/.lc1/.lc5` 编码器需要整数手，应改用 `KlineResp.Klines()`：

```go
r, err := c.GetKlineDay("sh000001", 0, 100)
if err != nil { return err }
ks, err := r.Klines()
if err != nil { return err }
fmt.Println(ks.VWAP())
```

`Klines()` 检查标准来源与旧数量上限，复制记录后按 `VolumeUnit` 适配；百股保持不变、股数除以 100，非整百股报错以避免截断。未知单位和扩展市场会报错。副本可用于标准指标与本地文件，转换和后续修改都不会污染原始协议记录。

`extend.DoIncomes` 按输入时刻所在的固定东八区日期选取标准日线，从该日零点开始，兼容本地日线零点和协议日线 15:00 时间标签；主机时区不再决定收益起始边界。

## 验证

离线管道测试覆盖代码解析、协议/周期路由、指数解码、混合报价排序与重复项、无需全局元数据的转债校准、分页边界、扩展列表过滤、失败传播、关闭与懒加载并发。港股日线复用 2026-10-09 原始响应及已保存的独立来源样本。新接口没有宣称所有扩展市场/字段完成准确性交叉验证。

```sh
go test -race . ./tests/... ./internal/... ./protocol/... ./extend/... ./lib/...
go test ./examples/... -run '^$'
```

验证产物和发布版本固定样本对照保存在 `output/unified-client/`，该目录不提交。

连接池新增 `GetContext(ctx)` 与 `DoContext(ctx, fn)`，可取消等待空闲连接；原 `Get/Do` 和 `ClientPool` 接口保持兼容。HTTP 标准和扩展行情路由都传入请求上下文，取消后不会继续排队或在借到连接后开始新查询。已开始的行情查询仍由客户端请求超时控制。

跨日完整分笔在一轮查询中共用日线日期集合；没有对应日线的日期仍跳过，日线校验失败仍返回错误。独立历史分笔查询继续进行日期校验。`update.Tracker.Mark(key)`（`lib/update`）可直接保存首次标记，`IsUpdated(key)` 查询不再创建占位行。
