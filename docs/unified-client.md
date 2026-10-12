# 统一行情 Client

`tdx.NewClient()` 返回统一业务接口 `tdx.Client`，默认实现为 `*tdx.MarketClient`。一个业务对象按证券代码自动选择标准行情或扩展行情，返回同一组 `Market*` 结果类型。原单连接结构体改名 `*tdx.Conn`，同样实现业务接口，原有 `Dial*` 与连接池继续使用；单连接的基础行情、全部 K线便捷方法及完整分笔查询统一为同名 `Get*` 和 `Market*` 返回类型。公开 `ExQuote/ExBars/ExMinute/ExHistMinute/ExTrade/ExHistTrade` 已移除，`ExQuoteList` 改名为 `GetQuoteList`；代码列表与数量使用统一 `GetCode/GetCodeAll/GetCount`，`ExMarkets` 改为 `GetMarkets`，`ExBarsRange` 收敛为 `GetKlineRange`，指数 K线查询并入 `GetKline*`；HTTP 路由及原响应形状保持兼容。

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

构造不联网，因此没有构造错误返回值；首次访问某类协议时建连，连接错误由查询方法返回。两类协议各复用一条连接，默认服务器和重连策略沿用 `DialDefault`/`DialExHqDefault`；`client.Option` 同时应用于两类连接。可并发查询；`SetTimeout` 覆盖当前和未来连接，默认仍为标准 2 秒、扩展 10 秒。`Close` 先释放已建立的连接，再等待正在进行的建连结束并释放晚到的连接，允许重复/并发调用，关闭后不会重新建连。两类连接各自控制初始化；一类建连变慢不会阻塞另一类查询或 `SetTimeout`。`Close` 仍需等待已有建连尝试结束，请求超时不控制建连时长。必须通过构造函数使用，零值不用于查询。

ETF、可转债等标准报价需要价格精度元数据时，使用同一连接取得所属交易所代码表，仅缓存在该业务对象的内存中，不依赖 `DefaultCodes`、数据库或额外连接。已缓存代码复用精度，缺少代码时重新读取对应市场；缺失精度信息会返回错误，不猜测价格。

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

业务变量和参数使用 `Client` 接口。`NewClient()` 按需连接两类协议；`Dial*()` 立即创建绑定一种协议的 `*Conn`，可赋给 Client，但不会自动建立另一类连接。`Client.Close()` 统一结束生命周期并停止重连，`SetTimeout()` 配置请求超时。原始 `SendFrame`、连接状态、日志、读取关闭信号等仅在 Conn 上提供；现有连接池继续使用 `*Conn`，不得改为指向接口的 `*Client`。完整迁移见 [v2 指南](v2-migration.md)。

## 查询接口

| 方法 | 返回 |
|---|---|
| `GetQuote(codes...)` | `[]MarketQuote` |
| `GetQuoteList(market, category, start, count)` | `[]protocol.ExQuoteListItem`，扩展市场分页列表 |
| `GetKline(category, code, start, count)` | `*MarketKlineResp` |
| `GetKlineMinute/5Minute/15Minute/30Minute/60Minute/Day/Week/Month/Quarter/Year` | `*MarketKlineResp` |
| `GetKlineAll/GetKlineUntil`，以及全部周期便捷方法的 `All/Until` | `*MarketKlineResp` |
| `GetKlineRange(code, startDate, endDate)`、`GetKlineMinute241Until(code, stop)` | `*MarketKlineResp` |
| `GetMinute(code)`、`GetHistoryMinute(date, code)` | `*MarketMinuteResp` |
| `GetTrade(code, start, count)`、`GetHistoryTrade(date, code, start, count)` | `*MarketTradeResp` |
| `GetTradeAll/GetHistoryTradeDay/GetHistoryTradeBefore/GetHistoryTradeFull` | `*MarketTradeResp` |
| `GetCode(exchange, start, count)`、`GetCodeAll(exchange)` | `*MarketCodeResp` |
| `GetCount(exchange)` | `int`，指定市场数量 |
| `GetMarkets()` | `[]MarketExchange`，市场发现结果 |

日期为有效 `YYYYMMDD`。K 线单次数量 1..800，实时/历史分笔单次数量分别为 1..1800/1..2000。K 线周期使用既有 `protocol.TypeKline*` 常量；`Day/Day2` 和 `Minute/Minute2` 在业务入口分别视作日线、一分钟别名，实际周期按市场配置映射，记录在返回的 `Category` 中。其他周期沿用协议编号，能否取得数据取决于服务器，不能把成功路由视为每个市场/周期已经通过实盘验证。

标准股票、指数和板块指数统一调用 `GetKline*`，内部自动选择股票/指数解码，不需要另选 `GetIndex*`。日线/一分钟的协议、分类与量单位配置下沉到 `protocol.Market.Info()`，`pull.Market` 等保留类型别名和原存储转换行为。

报价先解析全部输入再发送，标准品种合并请求，扩展品种顺序请求；结果保留原输入顺序与重复项。任何连接、请求或响应品种校验失败返回错误，不返回部分报价；标准请求超过 `uint16` 帧长上限会提前报错。混合结果不保证来自同一瞬间。

K 线分页按从近到远查询，返回列表为旧到新，`Until` 包含首次命中的边界记录。偏移超过 65535 返回错误，不回绕、不把部分历史当完整结果。返回 `Count` 为 `int`，不沿用旧响应总数的 `uint16` 限制。

代码分页 `start` 为 `uint32`，表示指定市场内偏移，`count` 为 `uint16`，有效范围 1..1000。标准沪深协议每次最多 1000 项，按请求数量截取；沪深偏移超过 65535 提前报错。扩展品种按全局序号分页，再按 `Exchange` 过滤，分页偏移作用于过滤后的列表；统一 `GetCount` 返回指定市场数量，因此扩展数量和分页查询均会扫描全局列表，暂不引入跨查询缓存。超出列表的页返回空列表。北交所沿用 `zhb.zip/tdxbjmore.cfg`，可能包括已分配未上市代码。列表不自动推断美股的股票/ETF/指数类别。

`GetMarkets` 的 `MarketExchange` 包含协议市场编号 `Market`、名称、简称和 `Protocol`；扩展原始分类等字段保留在 `Extended`。标准单连接返回内置沪深北，扩展单连接返回服务器市场表；跨市场入口合并两者，只需建立扩展连接，发现失败不返回部分结果。发现市场不意味着对应行情路由已接入。旧扩展全局计数和全局列表分页可使用 `SendFrame` 与 `protocol.MEx.FrameCount/FrameInstrument`，历史 HTTP 接口及 pull 共用快照保持原语义。

`GetKlineRange` 固定查询一分钟区间，日期包含两端；扩展使用原固定周期 7 的区间协议（不映射为普通扩展分钟周期 8），结果原字段保留在 `RangeExtended`，量单位和金额仍保持未确认。标准通过分页筛选，超出偏移上限报错。服务器可能限制历史可用范围；请求成功不保证长期完整覆盖。`GetKlineMinute241Until` 保留标准股票/ETF 集合竞价拆分，其他品种报错，不套用 241 根的交易时段。

完整分笔按递增偏移查询，较早页放在结果前面；偏移超过 65535、响应超出请求数量或请求失败均报错。跨日 `Before/Full` 需要实现 `TradingCalendar.Range` 的对应市场日历，沪深可继续用 `*Workday`；不自动用沪深日历判断港美股或期货交易日。每条历史分笔的 `Date` 为请求交易日期，夜盘自然日和未知价格尺度仍不推断。查询始于服务器最早可获取月线所在月份，不保证上市以来的全部历史。

历史分笔 `GetHistoryTrade/GetHistoryTradeDay` 在请求前查询同代码、同交易日期的日线；`Before/Full` 逐日沿用此检查。没有对应日线时不发送分笔请求，返回保留证券及日期的空响应（`Count=0`）；日线查询失败直接返回带代码、日期的错误。整日分页仅在首个分笔请求前检查一次，单页查询每次检查，包括非零偏移。检查只要求日期存在，不以日线成交量判定；当天实时 `GetTrade/GetTradeAll` 保持原行为。较早日期可能需要多页日线查询；服务器不再保留对应日线时，即使分笔接口非空也会跳过，日线存在也不保证分笔内容准确。

## 数据契约

- 统一价格为 `float64`，使用证券原币种/报价单位；标准整数厘通过现有 `Float64()` 转换，扩展浮点价格保留底层精度。没有汇率换算，也不把美元、港币或指数点标为人民币元。
- 成交量保留解码数值，每个接口独立标注 `VolumeUnit`：`hundred_shares` 为协议百股，`shares` 为股，`unknown` 为单位尚未确认。`unknown` 不乘 100。当前已配置的标准股票/ETF K 线为百股、标准指数/板块 K 线为股、港股日线为百股；可转债及未确认扩展周期为 unknown。报价、分时、分笔均保守标为 unknown，不套用 K 线单位配置。
- K 线 `Amount` 为指针，标准金额有值；扩展金额与持仓字段重叠，统一金额暂为 nil。期货持仓、结算价等保留在 `Extended` 中。原始结构的字段仍按各自既有契约解释，存在字段不代表其语义在全部市场已确认。
- 分笔公共 `Direction` 为 1 买、-1 卖、0 中性。扩展分笔的原始整数价格尺度未确认，公共 `Price` 为 nil，原始值保留在 `Extended.Price`；不会擅自除以 100/1000。[pytdx 上游分笔解析](https://github.com/rainx/pytdx/blob/master/pytdx/parser/ex_get_transaction_data.py) 同样保留整数原始价格。
- 每条数据的 `Standard` 或 `Extended` 保留完整的对应底层结构，避免丢失涨跌家数、均价、持仓、增仓、开平性质等特有信息。
- K 线 `Time` 为源数据墙上时间 `YYYY-MM-DD HH:MM`，分时/分笔为 `HH:MM`/`HH:MM:SS`。不生成 UTC 时间戳，不猜测美股时区、夜盘自然日或交易日；历史返回 `Date` 为请求日期，当日查询 `Date` 为空。标准当日分时仍沿用底层按运行环境当前日期请求历史分时的行为。

`Dial*` 返回的单连接 `Conn` 与 `NewClient()` 门面共享基础行情与代码查询的名称、参数和返回结构。单连接绑定建连时的协议，查询另一类品种会返回协议不匹配错误，不会额外建连；混合市场查询使用 `NewClient()`。K线全部周期及 `All/Until`、完整分笔查询均返回统一类型；指数 K线自动选择解码，独立 `GetIndex*` K线入口已移除。`Until` 回调接收 `MarketKline`，标准跨页昨收保留在原始 `Standard.Last`。财务、除权、公司资料、集合竞价、板块及报表等标准能力已接入业务接口，门面自动选择标准连接；扩展品种和扩展单连接不支持这些接口。它们保持原标准返回类型与单位。`Market*` 结果不是 `protocol.Klines`，不能直接传给现有指标、复权或存储方法；需要这些标准能力时调用 `StandardResponse()` 获取原始 protocol 响应；它保留整数价格和时间，不转换扩展数据，并检查旧 uint16 数量上限。

## 验证

离线管道测试覆盖代码解析、协议/周期路由、指数解码、混合报价排序与重复项、无需全局元数据的转债校准、分页边界、扩展列表过滤、失败传播、关闭与懒加载并发。港股日线复用 2026-10-09 原始响应及已保存的独立来源样本。新接口没有宣称所有扩展市场/字段完成准确性交叉验证。

```sh
go test -race . ./protocol/... ./extend/... ./lib/...
go test ./example/... -run '^$'
```

验证产物和发布版本固定样本对照保存在 `output/unified-client/`，该目录不提交。
