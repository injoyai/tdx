# v2 迁移指南

模块路径为 `github.com/injoyai/tdx/v2`，根包名仍为 `tdx`。下面的旧入口已移除，不再提供兼容别名。

## Client 接口与单协议连接

`tdx.Client` 是统一业务接口，`NewClient()` 返回按市场路由的 `*MarketClient`。标准 `Dial/DialDefault/DialWith/DialHosts*` 直接返回 `*StConn`；`DialEx/DialExDefault/DialExHosts` 直接返回 `*ExConn`。两种连接均实现 `Client`，单协议连接不会建立另一类连接。

公开 `Conn` 已移除。内部 `conn` 仅负责 TCP 收发、请求关联、超时及关闭/重连，不实现 `Client`，也不包含业务查询或协议专用解码。标准业务和解码归 `StConn`，拓展归 `ExConn`；共享分页、日期检查等通过绑定业务对象的 `queries` 执行，不再经由传输层绕回 `MarketClient`。

| 旧调用或类型 | 当前调用或类型 |
|---|---|
| 业务参数 `*tdx.Client` 或 `*tdx.Conn` | `tdx.Client` |
| 标准连接变量 `*tdx.Conn` | `*tdx.StConn`，由标准 `Dial*` 直接取得 |
| 拓展连接变量 `*tdx.Conn` | `*tdx.ExConn`，由 `DialEx*` 直接取得 |
| `NewStConn(conn)` / `NewExConn(conn)` | 已移除，直接使用对应 `Dial*` 返回值 |
| `NewPool` 工厂 `func() (*tdx.Conn, error)` | `func() (tdx.Client, error)` |
| `IPool.Get/Put` | 获取、归还 `tdx.Client` |
| `IPool.Do/Go` 回调 `func(*tdx.Conn)` | `func(tdx.Client)`，池中对象通过统一接口查询和关闭 |
| `CloseAll()` | `Close()`，结束整个生命周期并停止重连 |
| 原始 `SendFrame`、连接状态、日志等 | 从 `*StConn` / `*ExConn` 使用；业务 `Client` 不包含这些能力 |
| Codes/Gbbq/Workday 的客户端注入与对应工厂 | 继续接受 `Client` 及 `func() (Client, error)` |

```go
var c tdx.Client = tdx.NewClient(tdx.WithRedial(false))
defer c.Close()
quotes, err := c.GetQuote("sz000001", "hk00700")
_ = quotes; _ = err

pool, err := tdx.NewPool(func() (tdx.Client, error) {
    return tdx.DialDefault(tdx.WithRedial(false))
}, 1)
if err != nil { panic(err) }
defer pool.Close()
err = pool.Do(func(c tdx.Client) error {
    _, err := c.GetKlineDay("sz000001", 0, 100)
    return err
})
```

Go 函数返回类型不支持协变，不能把 `DialDefault` 直接当作 `func() (Client, error)` 传入；使用上述闭包适配。业务接口包装空指针仍按 nil 处理，连接池工厂返回 nil 或空指针客户端时报错并清理已初始化对象。

`Client.Close()` 保持关闭已有及晚到连接、停止重连的语义，可重复调用；传输层不再提供 `CloseAll` 兼容入口。财务、F10、集合竞价、除权、板块、报表和标准分类代码列表由 `StConn` 实现；`ExConn` 对这些接口明确报错。门面自动取得标准连接，标准接口的返回类型和单位保持不变。

## 跨市场统一查询

单协议业务连接实现使用简名 `StConn` / `ExConn`，此前 `StandardClient` / `ExtendedClient` 不保留别名；直接由对应 `Dial*` 创建。

统一返回对象去掉 `Market` 前缀，名称对应如下；`Client`、`MarketClient` 保持现有名称。

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

`tdx.NewClient()` 返回业务接口 `tdx.Client`，默认跨市场实现为 `*tdx.MarketClient`，按代码自动选择标准/扩展连接，并通过统一的 `GetQuote/GetKline*/GetMinute/GetTrade/GetCode/GetCodeAll/GetCount/GetMarkets` 返回统一结果。构造不联网，错误由首次查询返回。`Dial*` 和连接池仍可使用；单协议连接的基础行情与代码查询已变更返回类型，须按下表迁移，HTTP 原路由与响应形状保持兼容；新结果不是旧 `protocol.Klines`，不能直接用于原指标/复权/存储 API。字段、单位及支持边界见 [统一 Client](unified-client.md)。

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

`DialDefault/DialExDefault` 仍只连接对应服务，通用 `Get*` 查询其他协议品种会报错；跨市场使用 `NewClient()`。`Client` 与 `MarketClient` 的全部 K线周期、`All/Until` 便捷方法均返回 `KlineResp`；`Until` 的回调参数改为 `Kline`。独立指数 K线查询和 `ExBarsRange` 均已移除，无兼容别名。统一代码路由仍只覆盖已接入的市场；此前直接用任意市场编号调用六组 `Ex*` 的代码，在未接入市场上需使用已有 `SendFrame` 与 `protocol.MEx.Frame*`，不能仅把市场编号替换成未知前缀。

完整成交查询 `GetTradeAll/GetHistoryTradeDay/GetHistoryTradeBefore/GetHistoryTradeFull` 统一返回 `*TradeResp`，支持两类协议。单日历史响应和逐条 `Trade.Date` 保留请求日期；跨日响应的整体 `Date` 为空，每条记录有自己的日期。分页顺序、协议偏移 65535 护栏及标准跨页昨收保持；中途失败不返回部分历史。`Count` 使用 int，原标准 uint16 结果需要显式转换。

历史分笔查询新增同代码、同交易日期的日线检查：没有对应日线时返回空分笔，日线查询失败时返回错误；整日分页仅检查一次，跨日查询逐日检查。调用方不能再依赖无对应日线时分笔接口的非空返回；当天实时分笔不受影响。此保护可能增加日线分页请求，详见 [统一 Client](unified-client.md)。

`GetHistoryTradeBefore/Full` 的交易日历参数改为 `TradingCalendar`（提供 `Range` 方法）。日历实现使用 `*workday.Workday`（`extend/workday`），它是中国市场历史日历；其他市场必须由调用方提供对应日历。查询从最早可获取月线所对应月份开始，不保证服务器保留上市以来的全部分笔；`Before` 沿用日历的日期结束边界，获取整日数据，不按结束时刻截断当天分笔。扩展夜盘日期和价格尺度不作猜测换算。

复权和旧 HTTP 结构可使用 `resp.StandardResponse()` 获取原 `protocol.KlineResp/MinuteResp/TradeResp`。它只提取原始字段，不从浮点价格重建整数，不将扩展字段冒充标准数据；扩展结果和超过 uint16 数量上限的结果会报错。它保留标准指数的股数，不能直接作为整数手输入交给 VWAP 或本地文件写入器；这些场景使用新增的 `KlineResp.Klines()`：

```go
r, err := c.GetKlineDayAll("sz000001")
if err != nil { return err }
ks, err := r.Klines()
if err != nil { return err }
vwap := ks.VWAP()
_ = vwap
```

`Klines()` 返回记录副本，标准股票/ETF 保持整数手，标准指数从股换为手，原始记录及 HTTP 响应不变。未知单位、缺少标准来源、旧数量上限或不能无损换算的股数会报错，不能把未知转债或扩展单位猜成手数。

`GetKlineRange(code,startDate,endDate)` 固定查询一分钟线，日期使用 YYYYMMDD，包含两端日期。扩展保留原区间协议的固定请求周期 7（与普通扩展一分钟查询的周期映射不同），持仓和结算价等字段在 `RangeExtended`，金额及量单位未确认时保持未知；标准通过分页筛选日期区间，超出可读取分页范围时报错。未接入的协议市场仍须使用 `SendFrame`。`GetKlineMinute241Until` 在两种 Client 上使用同一签名及统一返回类型，仅适用于标准股票/ETF；指数或扩展品种明确报错，普通分钟查询使用 `GetKlineMinuteUntil`。

代码查询在 `Client` 与 `MarketClient` 上使用相同签名：`GetCount(exchange) (int, error)`、`GetCode(exchange, start uint32, count uint16) (*CodeResp, error)`、`GetCodeAll(exchange) (*CodeResp, error)`。原标准 `GetCode(exchange, start)` 需增加 `count`（旧分页大小为 1000）；`code.Code` 改为 `code.Instrument.Code`，小数位等特有元数据从 `code.Standard` 读取。分页 `count` 为 1..1000；扩展协议需扫描全局列表再按市场过滤，偏移按过滤后的列表计算。北交所数量和分页使用与完整列表相同的报表来源。

`GetMarkets()` 在标准连接上返回内置沪深北，在扩展连接上返回服务器市场表；`NewClient().GetMarkets()` 合并两者，并在扩展发现失败时返回错误。服务器列出的市场不一定已接入统一行情查询。需要旧扩展全局总数或全局分页时，使用既有 `SendFrame(protocol.MEx.FrameCount())` / `SendFrame(protocol.MEx.FrameInstrument(start, count))`，不能用任意单市场查询替代。历史 HTTP `/ex/count`、`/ex/instruments` 继续通过协议层保持原全局语义。pull 的全局品种快照入口改名 `Service.GetInstruments()`，本轮缓存范围保持不变。

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
| `extend.NewPullKline` / `PullKlineConfig` / `PullKline` | `pull.New` / `pull.Config` / `pull.Service` |
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
| `extend.DefaultDayKlineExportTitle` / `DefaultMinuteKlineExportTitle` | CSV 导出使用 `KlinesToCSV`；自定义列由调用方定义 |

`protocol.Klines` 成为唯一指标实现。VWAP 的输入成交量须为手，成交额须为厘；`pull` 查询结果使用股和元，需要先转换。标准行情指数日线的解码成交量为股，调用 VWAP 前也需转为手。VWAP 修正手股换算，ATR 使用最近 n 个真实波幅，RSI 在纯上涨窗口返回 100、平盘窗口返回 0。RSI/ATR 为已有的简单滚动窗口口径，不改为其他平滑算法。

第三方爬虫使用的 `extend.Factor/Factors` 继续保留，它们是数据源返回的比例模型，不等同于通达信仿射复权模型。

## 请求等待与超时

`SendFrame` 先注册回调再发送，避免快速响应早于等待注册而被丢弃；回调注册与响应处理互斥，超时或写入失败会清理回调与请求缓存，迟到响应会被忽略。标准行情默认请求超时仍为 2 秒，扩展行情仍为 10 秒，覆盖排队、发送和等待响应全过程。发送中途超时或短写会关闭当次连接，避免残缺帧污染后续请求；仅排队或等待响应超时则保留连接。

标准与扩展行情的握手、心跳、业务请求和重连初始化共用写入门控，保护底层连接状态与写入统计；新连接先完整发送握手，再放行业务写入。具体连接的 `Write` 及其字符串等写入方式保持稳定入口。心跳绑定当次连接。`Close()` 取消连接生命周期、关闭传输并等待读取流程退出后释放底层资源，门面还会取消进行中的首次建连和重试。截断或非法的成交、分时、股本和 F10 响应直接返回解码错误；压缩帧最多读取声明解压长度加一字节，超长响应直接报错。

扩展行情响应的头部或声明的记录不足时返回 `io.ErrUnexpectedEOF`，不再返回部分记录或把损坏响应当成空结果；合法的零记录响应仍正常返回。HTTP 服务的 `Close()` 同时释放自建的标准与扩展行情连接池，初始化失败也会清理已建连接池。

TCP 传输使用幂等关闭封装，避免重复关闭导致底层库跳过断开回调与缓存清理；`Origin()` 返回值仍支持 `net.Conn` 接口，调用方应依赖接口而非断言具体的 `*net.TCPConn` 类型。

真实只读验证按 `PROJECT_RULES.md` 第 8.5 节执行。标准与扩展行情测试共用每秒一次、最多 1000 次的写入限流，可用 `TDX_STD_LIVE=1 TDX_EXHQ_LIVE=1 go test -race . -run '^(TestStandardLive|TestExLive)$' -count=1 -v -timeout=150s` 启用；分别用 `TDX_STD_HOST` / `TDX_EX_HOST` 指定服务器 IP 或 `IP:端口`。标准测试只做一次受控重连，普通离线测试跳过真实访问。HTTP 上游验证用 `TDX_HTTP_LIVE=1 go test -race ./extend/httpserver -run '^TestServerLive$' -count=1 -v -timeout=45s`，须在行情测试完成后单独执行，不并行启动。

数据准确性还须按项目规则第 8.6 节对照独立来源，其他来源单线程、请求起始间隔至少 5 秒。`TDX_CROSS_LIVE=1 go test -race . -run '^TestCrossValidationSamplesLive$' -count=1 -v -timeout=45s` 保存小样本到 `output/live-validation/cross/tdx-samples.json`，供另行串行获取独立数据并逐字段比较；该测试成功只表示样本获取成功。港股主板日线现用 category=4，腾讯控股样本的 `ExKline.Trade` 为百股，×100 与独立来源在整数百股精度内一致；不能将该单位推广到其他扩展接口。小时线的来源聚合差异和港股分时的时间标签仍须确认。

标准行情代码、K 线与五档报价也检查记录边界和变长整数；截断或溢出响应直接返回解码错误。`protocol.MQuote.Decode` 保留原返回类型，异常数据返回 `nil`；需要错误详情时改用 `protocol.MQuote.DecodeChecked`。

`Hosts`、`ExHosts` 及地域子列表类型改为 `Addrs`（底层为 `[]string`），统一存储 `IP:端口`，从列表取得地址后不要再拼接端口。`NewExRangeDial` 已移除，使用 `NewRangeDial(tdx.ExHosts)`；自定义扩展列表必须显式带端口，若需要保留裸 IP 默认补 `7727` 和空列表使用 `ExHosts` 的行为，直接使用 `DialExHosts(hosts)`。`Dial` / `DialHosts*` 仍兼容裸 IP，默认补 `7709`；`DialEx` / `DialExHosts` 默认补 `7727`，显式端口均保留。

默认地址列表也为空时，拨号返回错误；扩展拨号不会回退到标准列表。将测速结果赋给默认列表后无可用地址时也适用此规则。

`NewTCPDial`、`NewHostDial`、`NewRandomDial`、`NewRangeDial` 只使用传入的完整地址，不补端口、不选择协议默认列表；列表为空时返回拨号错误。原 `New*Dial(nil)` 选择标准列表的用法须改为显式传入 `tdx.Hosts`。需要兼容裸 IPv4/IPv6 和空列表选择协议默认地址时，使用 `Dial` / `DialHosts*` 或 `DialEx` / `DialExHosts`。完整 IPv6 地址使用 `[IPv6]:端口`，裸 IPv6 可以带 zone，如 `fe80::1%eth0`。

四个 `New*Dial` 都使用调用方的 context 建连，每个地址最多等待 5 秒；遍历地址间的退避等待也可取消。轮询和随机拨号函数可以在多个客户端之间并发复用。

测速实现移入 `Addrs.Sort(timeout ...time.Duration) Addrs` 方法，`SortHosts` / `SortExHosts` 已移除。原 `SortHosts(hosts, timeout...)` 改为 `tdx.Addrs(hosts).Sort(timeout...)`；标准和扩展内置列表直接使用 `tdx.Hosts.Sort(timeout...)` / `tdx.ExHosts.Sort(timeout...)`。地址须包含端口；方法返回新的 `Addrs`，不修改输入或全局默认列表。需要保留原更新默认列表的行为时显式赋值：

```go
tdx.Hosts = tdx.Hosts.Sort()
tdx.ExHosts = tdx.ExHosts.Sort()
```

`Addrs` 可直接传给现有接收 `[]string` 的拨号函数，无需转换或泛型；普通 `[]string` 也可以赋给默认列表。若外部代码使用反射或断言检查默认列表的具体类型，需要同步改为 `Addrs`。

超时是整轮测速的时间上限（默认一秒），按 TCP 连接耗时排序；全不可用或输入为空时返回空列表，不使用默认地址，也不无限等待第一个可用地址。测速仅检查 TCP 建连，不代表行情握手或查询成功。

### 管理器独立子包

管理器迁移到 `github.com/injoyai/tdx/v2/extend/manage`，构造入口为 `manage.New(...)`，类型为 `*manage.Manage`。这是公开接口迁移，调用方需要更新 import、构造函数、类型和选项的包名前缀；根包不再提供管理器入口，避免反向依赖扩展包造成循环引用。

| 原入口 | 新入口 |
|---|---|
| `tdx.NewManage(...)` | `manage.New(...)` |
| `*tdx.Manage` | `*manage.Manage` |
| `tdx.Option` | `manage.Option` |
| `tdx.WithClients` / `tdx.WithPool` / `tdx.WithDialPool` | 同名 `manage.With*` |
| `tdx.WithCodes` / `tdx.WithDialCodes` | 同名 `manage.With*` |
| `tdx.WithWorkday` / `tdx.WithDialWorkday` | 同名 `manage.With*` |
| `tdx.WithGbbq` / `tdx.WithDialGbbq` / `tdx.WithDialGbbqDefault` | 同名 `manage.With*` |
| `tdx.WithOption` | `manage.WithOptions` |

```go
import "github.com/injoyai/tdx/v2/extend/manage"

m, err := manage.New(manage.WithClients(2))
if err != nil {
    return err
}
defer m.Close()
```

`pull.Config.Manage`、`Service.Manage()` 和 `extend.PullTrade` 的管理器参数同步改为 `*manage.Manage`。代码接口、模型和缓存工厂位于 `extend/codes`；股本缓存位于 `extend/gbbq`；连接池和 `tdx.DialClientFunc` 仍在根包，数据库工厂为 `xorms.DialFunc`。代码缓存配置使用 `codes.WithClient` 等入口；交易日历已迁至 `extend/workday`，更新标记及定时工具迁至 `lib/update`。缓存路径、自动更新及资源归属不因迁移改变。用法见 [manage 包说明](../extend/manage/README.md)。

`manage.Manage.Close()` 先停止工作日任务调度，按创建顺序的逆序释放内部缓存和连接池，再等待正在执行的任务退出，避免任务等待空连接池时互相阻塞；初始化失败也执行相同清理。关闭没有强制截止时间，任务自身须有界退出；外部共享连接池的阻塞由调用方解除。`WithPool` / `WithCodes` / `WithWorkday` / `WithGbbq` 直接注入的资源仍由调用方关闭；相应 `WithDial*` 工厂返回的资源归管理器所有。pull 懒加载的股本缓存也由管理器释放。旧 `extend.ListenCodesHTTP` / `ListenCodesAndGbbqHTTP` 初始化或监听失败会返回错误并关闭自建缓存；两者共用按 URL 路径匹配的路由，支持查询参数，保持原始 JSON 列表/映射返回。

`manage.New` 在代码、交易日历和股本组件均已提供时不借用连接；需要初始化组件时才从池中获取连接。`WithDialPool` / `WithDialCodes` / `WithDialWorkday` / `WithDialGbbq` 返回空对象（包括接口中的空指针）会导致构造失败并清理已创建的资源。`InitGbbq()` 只替换管理器自己的默认占位对象；通过 `WithGbbq` 或 `WithDialGbbq` 提供的空实现也保持原样，可用于明确禁用股本回填。

`Close()` 会等待正在执行的工作日任务，不能在 `AddWorkdayTask` 的回调内同步调用。任务需要关闭管理器时，向外部调用方发送信号并返回，由外部调用 `Close()`：

```go
shutdown := make(chan struct{}, 1)
err := m.AddWorkdayTask("0 0 16 * * *", func(*manage.Manage) {
    select {
    case shutdown <- struct{}{}:
    default:
    }
})
if err != nil {
    return err
}
<-shutdown
return m.Close()
```

### 代码表接口与缓存独立子包

`Interface`、代码模型、内存代码表和 SQLite 缓存迁移到 `github.com/injoyai/tdx/v2/extend/codes`。查询方法、模型字段、`codes` 表名、HTTP JSON 形状及默认缓存路径保持原样；需要同步更新调用方的包名前缀。

| 原入口 | 新入口 |
|---|---|
| `tdx.ICodes` / `tdx.Codes` | `codes.Interface` / `codes.Codes` |
| `tdx.CodeModel` / `tdx.CodeModels` / `tdx.CodesBase` | `codes.CodeModel` / `codes.CodeModels` / `codes.Base` |
| `tdx.NewCodes` / `tdx.NewCodesSqlite` / `tdx.NewCodesBase` | `codes.New` / `codes.NewSQLite` / `codes.NewBase` |
| `tdx.CodesOption` / `tdx.DialCodesFunc` | `codes.Option` / `codes.DialFunc` |
| `tdx.WithCodesDB` / `tdx.WithCodesDialDB` | `codes.WithDB` / `codes.WithDialDB` |
| `tdx.WithCodesSpec` / `tdx.WithCodesRetry` | `codes.WithSpec` / `codes.WithRetry` |
| `tdx.WithCodesClient` / `tdx.WithCodesDialClient` | `codes.WithClient` / `codes.WithDialClient` |
| `tdx.WithCodesOption` / `tdx.DefaultCodes` | `codes.WithOptions` / `codes.DefaultCodes` |

`manage.WithCodes` 接受 `codes.Interface`，`manage.WithDialCodes` 接受 `codes.DialFunc`；HTTP 缓存服务的配置参数改为 `codes.Option`。组件注入与工厂资源的归属规则保持不变。`Interface` 只表达代码表读取能力，不强制内存代码表提供关闭方法；需要释放资源时关闭具体缓存对象。

标准行情报价仍在同一连接上取得精度元数据，使用内部的纯内存代码表，不依赖 `codes.New`、全局默认缓存或数据库。内部共享实现也用于对外的 `codes.Base`，避免两份代码分类与价格换算逻辑。元数据刷新和关闭的同步机制由 `internal/lifecycle` 共享，保持关闭等待在途更新的行为。

`codes.Codes`、`gbbq.Gbbq`、`workday.Workday` 使用完成后须调用 `Close()`，停止定时更新并释放自建客户端和数据库。关闭可重复或并发调用，会等待在途更新；关闭后 `Update()` 返回 `net.ErrClosed`，已有内存快照仍可查询。`codes.New`、`gbbq.New`、`workday.New` 初始化失败返回 nil 和错误，并释放本次通过工厂创建的客户端及数据库；`With*Client` / `With*DB` 直接注入的共享资源仍由调用方管理。相应 `With*DialClient` / `With*DialDB` 工厂应返回本次构造独占的资源。HTTP 股本查询和本地查询统一按记录日期的零点生效，协议记录中的 15:00 不作为生效时刻。

### 交易日历与更新工具独立子包

数据库、行情拉取和定时刷新组成的历史交易日历迁至 `github.com/injoyai/tdx/v2/extend/workday`。根包继续保留 `TradingCalendar` 抽象和 `IntegerDay` 日期工具；历史分笔查询不再依赖某个具体日历类型，接口中的空日历指针会在读取行情前返回错误。

| 原入口 | 新入口 |
|---|---|
| `tdx.Workday` / `tdx.WorkdayModel` | `workday.Workday` / `workday.WorkdayModel` |
| `tdx.NewWorkday` / `tdx.NewWorkdaySqlite` | `workday.New` / `workday.NewSQLite` |
| `tdx.WorkdayOption` / `tdx.DialWorkdayFunc` | `workday.Option` / `workday.DialFunc` |
| `tdx.WithWorkdayClient` / `tdx.WithWorkdayDialClient` | `workday.WithClient` / `workday.WithDialClient` |
| `tdx.WithWorkdayDB` / `tdx.WithWorkdayDialDB` | `workday.WithDB` / `workday.WithDialDB` |
| `tdx.WithWorkdaySpec` / `tdx.WithWorkdayRetry` / `tdx.WithWorkdayOption` | `workday.WithSpec` / `workday.WithRetry` / `workday.WithOptions` |

`manage.Workday` 与 `pull.Config.Workday` 同步使用 `*workday.Workday`；`manage.WithDialWorkday` 的工厂类型为 `workday.DialFunc`。数据库 `workday` 表、默认路径 `./output/data/database/workday.db`、固定东八区和历史交易日判断方式保持原样。此实现依赖上证指数已经返回的日线，不提供未来节假日预测，也不自动成为港美股或期货的交易日历。

持久化更新标记与定时执行移至 `github.com/injoyai/tdx/v2/lib/update`，分别位于 `mark.go` 和 `timer.go`。该公共工具包不依赖根包，供行情元数据、扩展缓存及 pull 共同使用。

| 原入口 | 新入口 |
|---|---|
| `tdx.Updated` / `tdx.UpdateModel` | `update.Tracker` / `update.Record` |
| `tdx.NewUpdated(db, hour, minute)` | `update.New(db, hour, minute)` |
| `tdx.Updater` / `tdx.StartTimer` / `tdx.NewTimer` | 同名 `update.*` 入口 |

`pull.Config.Tracker` 改为 `*update.Tracker`。更新标记沿用 `update` 表名、字段和每日边界判断方式；`update.New` 使用调用方传入的数据库，不接管其关闭。新包用法见 [workday](../extend/workday/README.md) 和 [update](../lib/update/README.md)。

独立调度使用 `stop, err := update.StartTimer(spec, retry, updater)`，成功后 `defer stop()`。它先验证 cron 表达式并立即更新一次，然后启动定时任务；同一任务不会重叠，停止会取消重试等待并等待正在执行的更新返回。更新函数本身须有有限超时。`update.NewTimer` 保留原签名，用于持续到进程退出的任务。

`extend.DialCodesHTTP` / `DialGbbqHTTP` 返回的缓存客户端也须 `Close()`，关闭会取消在途 HTTP 请求并停止更新。外部 HTTP 数据请求（代码/股本缓存、同花顺、新浪和股本文件下载）默认总超时 30 秒；非成功状态、畸形同花顺响应，以及新浪复权因子中的空记录、非法数字、NaN 或无限值均返回错误。

`lib/zip.Encode` 拒绝源文件与目标文件相同的路径、硬链接或符号链接，检查失败不会截断源文件；压缩写入及关闭错误会返回调用方。

内置 pull 代码复用协议层校验：扩展代码最多 9 字节，美股须含字母，期货须同时含字母和数字，完整市场键须匹配品种。过去被接受的非法配置现在提前返回错误；合法代码沿用原文件名，自定义市场保留自己的代码规则。`Instrument.Market()` 将沪市 880/881 板块归为 `cn/block`，不再误归 `cn/index`。

边界行为修正：`Merge241(n<=0)` 原样返回；`DoIncomes` 忽略负偏移，起始日期晚于全部数据时返回空。pull 的 `DayToPeriod` / `MinuteToPeriod` / `DayToCalendar` 直接合并原始浮点量价，保留超过三位小数及负价格；不再往返整数厘。裸美股代码统一转大写参与去重和文件定位。旧的小写美股库不会自动重命名，使用前应在停机备份后合并到大写文件名，包含相应 WAL/SHM；不要直接覆盖已有大写库。


`extend.WriteMinute1` 仅在当日包含 15:00 收盘记录时补尾盘占位，盘中或部分日的数据原样编码；已有占位不会重复补入。

代码元数据刷新使用 SQLite 增量事务，保留未变化记录和原主键，更新名称、倍数、小数位与昨收价，删除退市项；内存缓存使用提交后的数据库快照。

设置请求超时统一使用 `Client.SetTimeout(duration)`；原先直接调用单连接 `Client.Wait.SetTimeout` 的代码需迁移到该方法。具体连接的 `Wait` 保留原类型，用于内部响应关联，直接修改它不会调整 `SendFrame` 的计时器。

## 成交行为

成交接口的参数、返回类型、分页顺序和请求协议保持一致；HTTP 服务的成交路由与响应格式保持一致。

标准行情的全部代码、成交、K 线分页使用整数累加偏移；偏移或累计数量超过 `uint16` 上限（65535）会报错，不会回绕或返回错误数量。此时调用方不应将结果视为完整历史。`GetBlockFileRaw` 按元信息校验文件长度，缺块返回 `io.ErrUnexpectedEOF`，过长分块也会报错。

成交示例统一见 [GetTrade](../examples/trades/GetTrade/main.go)。

北交所代码通过通达信 `zhb.zip` 中的 `tdxbjmore.cfg` 获取，返回 `CodeResp.List`，包含代码与名称。需要 `bj` 前缀字符串时，将 `"bj"` 拼接到每条记录的 `Code`；该配置可能包含已分配但尚未上市的代码，不等同于官网上市证券列表。示例见 [GetCodeAll](../examples/codes/GetCodeAll/main.go)，使用 `-exchange 2 -all`。

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
| `MinKlines(code, start, end)` | `s.QueryMinute(c, start, end)` |

`Dir` 必须显式设置；`StartAt` 改用 `YYYYMMDD` 字符串，未设置时默认最近两年，非法格式或不存在的日期在创建服务时直接报错，且不会创建数据库。`Day` 与 `Minute` 都为 `false` 时默认同时拉取两种数据。服务使用完成后调用 `s.Close()`，外部连接池仍由调用方关闭。

日线查询返回 `[]*pull.KlineDay`，分钟查询返回 `[]*pull.KlineMinute`；价格与已确认金额使用证券原币种或报价单位，已确认单位的股票/ETF/指数/板块及港股日线成交量为股。新增 `AmountUnknown` 列区分未知金额和真实零值；内置扩展适配器写入 `Amount=0, AmountUnknown=true`。旧库通过原有 Sync2 流程加列，查询内置扩展市场的历史金额也标为未知并返回 0，不批量覆盖旧金额；直接 SQL 访问旧列仍须自行识别这些旧值。聚合包含未知金额时，金额也返回未知。将标准价格交给使用厘单位的 `protocol` API 时需转换单位。示例见 [PullV2](../examples/data/PullV2/main.go) 和 [Income](../examples/finance/Income/main.go)，完整服务说明见 [pull 文档](../extend/pull/README.md)。

内置市场的 K 线参数和成交量换算现集中于 `Market.Info()`；由 `ParseCode` 得到市场后可直接查询。未知单位设为 `VolumeDefault`，暂按旧倍率 ×100，不能据此认定这些扩展市场的量单位为股或合约。配置只覆盖日线和 1 分钟线。

港股主板日线由 category=9 改为 4，修正成交量请求口径；标准行情指数及板块分钟线直接使用解码后的股数，取消拉取层重复 ×100。已有数据库不会自动修正历史记录；旧港股日线首条的异常量不能靠整体乘倍数恢复，重新获取时请保留旧库，并考虑服务器历史保留范围。

## 数据库支持

内置数据库支持统一使用 SQLite，已移除 MySQL 驱动注册、直接依赖和专用示例。以下入口不再提供：

| 已移除的入口 | SQLite 用法 |
|---|---|
| `tdx.NewManageMysql` | `manage.New`（`extend/manage`） |
| `tdx.NewCodesMysql` | `codes.New` / `codes.NewSQLite`（`extend/codes`） |
| `tdx.NewWorkdayMysql` | `workday.New` / `workday.NewSQLite` |
| `xorms.NewMysql` | `xorms.NewSQLite(filename)` |

通过 `codes.WithDB`、`workday.WithDB`、`gbbq.WithDB` 或对应 `DialDB` 选项传入 SQLite 引擎，可自定义数据库路径。已有 MySQL 数据不会自动转换为 SQLite；需要保留数据时另行导出转换，不能直接将 MySQL DSN 当作 SQLite 文件名。

## 存量数据库

旧服务使用 `day-kline/`、`min-kline/` 与 `update.db`；新服务按市场存储，例如 `cn/stock/day/sz000001.db`、`cn/stock/min/sz000001/sz000001-2026.db`，去重库为 `updated.db`。

旧表与新表的字段结构、时间字段和量价单位不同，不能仅重命名目录直接复用。新服务不会自动读取或转换旧服务数据库；需要单独转换存量数据，或在新目录重新拉取。重新拉取只能恢复服务器仍可提供的历史，旧库应保留以便转换。

新服务自身已有 SQLite 表的补字段、覆盖标记兼容继续保留，与上述旧服务目录迁移是两回事。

## 默认缓存目录

Go 最低版本为 1.25.0。默认数据目录从 `./data` 调整为 `./output/data`，代码、交易日和股本缓存使用 `./output/data/database`；分笔 CSV 示例输出到 `./output/trade`。

旧缓存不会自动搬迁。需要保留现有代码/交易日/股本缓存时，关闭所有使用它们的进程后，将 `codes.db`、`workday.db`、`gbbq.db` 及对应 SQLite WAL/SHM 文件完整复制到新目录，保留原文件备份；这些缓存表结构不变。也可以通过 `codes.WithDB` / `workday.WithDB` / `gbbq.WithDB` 或对应 `DialDB` 选项显式配置路径。这类元数据缓存迁移与上面的旧 K 线数据库格式转换不同。


## 固定东八区与旧库隔离

新行情时间统一使用固定 `UTC+08:00`（`protocol.Location()`），不再依赖运行机器的 `TZ`。标准 K 线和股本协议的 `time.Time`、标准当日查询日期、交易日判断、`update.StartTimer` / `manage.Manage.AddWorkdayTask` 的 cron 时区及更新去重节点都使用东八区。统一 API 的字符串仍保留源钟面值；扩展数据进入 pull 时也按固定东八区编码，不据此声称其为证券当地的真实 UTC 时刻。

pull 的新数据目录带 `.storage-timezone` 标记。无标记且已有 `.db` 的目录在 `New` 中被拒绝，不会写入、移动或清理原库。请先使用全新的 `Config.Dir`；旧库另行迁移，本次不提供自动迁移。即使原机器一直处于东八区，也应先验证旧库口径，再由单独迁移步骤建立标记，不能通过手工补标记绕过检查。

迁移前停用所有写入者并备份 SQLite 文件及 WAL/SHM。必须先确认旧库每类数据原先采用的时区；仅有 Unix 数值不足以恢复未记录的时区。转换时需同步处理日线/分钟 Unix 主键、年份文件归属、分钟覆盖区间和缓存标记；去重与空扫描标记应在独立迁移中重新认证。代码、交易日、股本缓存中的旧时间也须核对；可重新获取的元数据优先在新缓存目录重建，保留旧缓存备份。迁移工具和已有数据的实际转换不包含在这次优化中。


### 重复逻辑收敛后的兼容边界

- `protocol.DecodeCode` 对已接入市场复用 `ParseInstrument`，港股指数前缀、点分隔前缀、交易所后缀和美股点代码使用同一规则。未接入统一 Client 的原始协议市场仍保留旧解析能力，不代表统一 Client 已支持该市场。
- pull 的内置代码解析也复用这套规则，`000001.SZ` 归深市、`BRK.B` 归美股；完整市场键及已注册自定义市场继续保留。`ParseCode` 不再将任意 `unknown.code` 静默当成市场；需要自定义市场时先注册 Unit。`SplitKey` 仍是兼容性的尽力拆分接口，服务初始化应使用有错误返回的 `ParseCode`。
- 标准分笔解码的钟面值现带固定东八区标签，修正原有 UTC 标签；日期与时分保持不变，Unix 值会相差八小时。`Trades.Klines` 保留输入交易日的时区，不再借用主机时区恢复日期。本地日线保持 00:00，协议日线/同花顺日线保持 15:00；同花顺金额按交易日期关联。
- `PullTrade` 的 5/15/30/60 分钟 CSV 使用 `Merge241`：09:30 竞价单独保留，后续按连续交易分钟聚合，保留 11:30/13:01 午休边界。记录数和总量额不变，首根及各周期终点修正。已有 CSV 如需统一口径，应另行重新导出。
- HTTP 标准分时、分页分笔和完整分笔统一调用 `StandardResponse()`，保留既有 JSON 字段；缺失标准原始记录时返回业务错误，不再输出 null 记录或触发 nil 解引用。`exchange` 参数复用 `ParseExchange` 后限制沪深北，支持混合大小写。
- `GetReportFile` 与 `GetBlockFileRaw` 共享单块校验，超长响应现在返回错误；前者仍按短块/空块结束，后者仍要求达到元数据声明的文件大小。
- `protocol.DecodeKline` 保留兼容并标注弃用；其旧五价格字段布局与 `MKline.Decode` 不同，不能直接替换输入字节。`lib/gbbq.GetStocks` / `Equity` / `Turnover` 的旧结果保留并标注弃用，文件单位未独立核实，暂不提供自动转换。

## 股本缓存与默认配置归位

股本缓存迁至 `github.com/injoyai/tdx/v2/extend/gbbq`，根包不再提供兼容别名。`Client.GetGbbq/GetGbbqAll` 仍是底层查询入口；`protocol` 中的数据结构、单位和复权算法保持不变。

| 原入口 | 当前入口 |
|---|---|
| `tdx.NewGbbq` / `tdx.Gbbq` / `tdx.IGbbq` | `gbbq.New` / `gbbq.Gbbq` / `gbbq.Interface` |
| `tdx.GbbqOption` / `tdx.DialGbbqFunc` | `gbbq.Option` / `gbbq.DialFunc` |
| `tdx.WithGbbqClient` / `tdx.WithGbbqDialClient` | `gbbq.WithClient` / `gbbq.WithDialClient` |
| `tdx.WithGbbqDB` / `tdx.WithGbbqDialDB` | `gbbq.WithDB` / `gbbq.WithDialDB` |
| `tdx.WithGbbqSpec` / `tdx.WithGbbqRetry` / `tdx.WithGbbqOption` | `gbbq.WithSpec` / `gbbq.WithRetry` / `gbbq.WithOptions` |
| `tdx.DialDBFunc` | `xorms.DialFunc`（`lib/xorms`） |
| `tdx.DefaultClients` | `manage.DefaultClients` |
| `tdx.DefaultCodesSpec` / `tdx.DefaultWorkdaySpec` / `tdx.DefaultGbbqSpec` | `codes.DefaultSpec` / `workday.DefaultSpec` / `gbbq.DefaultSpec` |
| `tdx.DefaultDataDir` / `tdx.DefaultDatabaseDir` | 改为内部路径配置，调用方通过各缓存 `WithDB` / `WithDialDB` 自定义路径 |

`tdx.DialClientFunc` 和通用 `tdx.DefaultRetry` 保留。默认路径仍为 `./output/data/database`，三个缓存默认刷新时间仍为固定东八区 09:01 / 09:03 / 09:05，无需搬迁数据库。HTTP 服务入口仍在 `extend`，其中股本参数改为 `[]gbbq.Option`，`GbbqHTTP` 实现 `gbbq.Interface`。管理器的股本注入和工厂分别接收 `gbbq.Interface`、`gbbq.DialFunc`；资源归属与懒初始化规则保持不变。`lib/gbbq` 仍用于离线文件解析，不能直接代替股本缓存。


## 元数据查询接口命名

独立子包的查询接口统一使用 `Interface`，具体持久化缓存类型保留原名：

| 原子包入口 | 当前入口 |
|---|---|
| `gbbq.IGbbq` | `gbbq.Interface` |
| `codes.ICodes` | `codes.Interface` |

`gbbq.New` 仍返回 `*gbbq.Gbbq`，`codes.New` 仍返回 `*codes.Codes`。接口方法集、工厂及资源归属语义保持不变；管理器、HTTP 和 pull 的类型声明已同步更新。旧接口名不保留兼容别名，调用方需替换类型声明和匿名嵌入字段名称。


## 字符转换与均单成交额名称修正

| 旧入口 | 当前入口 |
|---|---|
| `protocol.UTF8ToGBK` | `protocol.GBKToUTF8` |
| `(*protocol.Trade).AvgPrice()` | `(*protocol.Trade).AvgAmount()` |

旧入口已移除，调用方需同步更新名称。`GBKToUTF8` 保持原来的 GBK 解码为 UTF-8、去除 NUL 字节的行为；没有反转转换方向。`AvgAmount()` 保持平均每单成交量乘成交价格、再换算百股单位的公式，返回成交额，单位仍为厘，`.Float64()` 得元。扩展分时记录 `ExMinuteTick.AvgPrice` 的均价字段及其 JSON 键 `avgPrice` 保持不变。


## 命名一致性整理

以下旧名称已移除，项目外的 Go 调用方、工厂签名、自定义 Unit 和匿名嵌入字段需同步更新：

| 原入口 | 当前入口 |
|---|---|
| `tdx.IPool` | `tdx.ClientPool`，具体实现仍为 `*tdx.Pool` |
| `tdx.DialExHq` / `DialExHqDefault` / `DialExHqHosts` | `tdx.DialEx` / `DialExDefault` / `DialExHosts` |
| `httpserver.WithExHqHosts` / `httpserver.WithOptions` | `httpserver.WithExHosts` / `httpserver.WithClientOptions` |
| `codes.WithOption` / `gbbq.WithOption` / `workday.WithOption` / `manage.WithOption` | 对应子包的 `WithOptions` |
| `xorms.NewSqlite` / `codes.NewSqlite` / `workday.NewSqlite` | 对应子包的 `NewSQLite`；缓存原有 `New` 仍可用 |
| `pull.NewService` | `pull.New` |
| `codes.CodesBase` | `codes.Base`，构造入口仍为 `codes.NewBase` |
| `update.Updated` / `update.UpdateModel` | `update.Tracker` / `update.Record` |
| 标记对象的 `Update(key)` / `Updated(key)` | `Mark(key)` / `IsUpdated(key)`；执行任务的 `Updater.Update()` 保持原名 |
| `pull.Config.Updated` | `pull.Config.Tracker` |
| `QueryMin` / `QueryMinCoverage` / `SaveMin` / `SaveMinComplete` / `SaveMinFallbackComplete` / `SaveMinEmptyScan` | `QueryMinute` / `QueryMinuteCoverage` / `SaveMinute` / `SaveMinuteComplete` / `SaveMinuteFallbackComplete` / `SaveMinuteEmptyScan` |
| `MinFile` / `MinExists` / `MinYearComplete` / `MinYearRecentlyEmpty` / `LastMinUnix` | `MinuteFile` / `MinuteExists` / `MinuteYearComplete` / `MinuteYearRecentlyEmpty` / `LastMinuteUnix` |
| `pull.Unit.FetchMin` | `pull.Unit.FetchMinute`，自定义 Unit 和调用方须同步改名 |
| `extend.KlinesToCsv` / `extend.TradeToCsv` | `extend.KlinesToCSV` / `extend.TradesToCSV` |
| `extend.UrlSinaFactorHfq` / `UrlSinaFactorQfq` / `UrlTHSDayKline` | `extend.URLSinaFactorHFQ` / `URLSinaFactorQFQ` / `URLTHSDayKline` |
| `extend.Sina_QFQ` / `Sina_HFQ` / `THS_BFQ` / `THS_QFQ` / `THS_HFQ` | `extend.SinaQFQ` / `SinaHFQ` / `THSBFQ` / `THSQFQ` / `THSHFQ` |
| `protocol.Quote.ReversedBytes0..9` | `protocol.Quote.Reserved0..9` |
| `protocol.Trades.Volume2()` | `protocol.Trades.SideVolumes()`，仍返回主动卖出和主动买入的股数 |

`update` 表的名称与 Key/Time 列、行情数据库表列、`updated.db`、`min/` 数据目录均不变。`Quote.Reserved0..9` 通过显式 JSON tag 保留旧 `ReversedBytes0..9` 键；`Config.Tracker` 的 JSON 键仍为 `Updated`，`Manage` 的匿名 `ClientPool` 字段 JSON 键仍为 `IPool`。行情字段的原始数值、单位、HTTP 路由与返回结构保持不变。此整理不为未知协议字段补充未经确认的业务含义。

源码文件统一使用小写下划线：`codes_server.go`、`pull_trade.go`、`spider_sina.go`、`spider_ths.go`，因子模型为 `extend/factor.go`；扩展 HTTP handler 为 `handler_ex.go`，传输写入为 `conn_writer.go`，ZIP 工具为 `lib/zip/zip.go`，对应测试同步。扩展连接示例路径改为 `examples/client/ExClient`；真实扩展测试名称改为 `TestExLive`，原 `TDX_EXHQ_LIVE` 环境开关保持不变。
