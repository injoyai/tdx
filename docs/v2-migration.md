# v2 迁移指南

模块路径为 `github.com/injoyai/tdx/v2`，根包名仍为 `tdx`。下面的旧入口已移除，不再提供兼容别名。

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

设置请求超时统一使用 `Client.SetTimeout(duration)`；原先直接调用 `Client.Wait.SetTimeout` 的代码需迁移到该方法。`Client.Wait` 仍保留原类型，用于内部响应关联，直接修改它不会调整 `SendFrame` 的计时器。

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

日线查询返回 `[]*pull.KlineDay`，分钟查询返回 `[]*pull.KlineMinute`；价格与金额为元，成交量为股。将价格交给使用厘单位的 `protocol` API 时需转换单位。示例见 [PullKline](../example/PullKline/main.go) 和 [Income](../example/Income/main.go)，完整服务说明见 [pull 文档](../extend/pull/README.md)。

## 存量数据库

旧服务使用 `day-kline/`、`min-kline/` 与 `update.db`；新服务按市场存储，例如 `cn/stock/day/sz000001.db`、`cn/stock/min/sz000001/sz000001-2026.db`，去重库为 `updated.db`。

旧表与新表的字段结构、时间字段和量价单位不同，不能仅重命名目录直接复用。新服务不会自动读取或转换旧服务数据库；需要单独转换存量数据，或在新目录重新拉取。重新拉取只能恢复服务器仍可提供的历史，旧库应保留以便转换。

新服务自身已有 SQLite 表的补字段、覆盖标记兼容继续保留，与上述旧服务目录迁移是两回事。

## 默认缓存目录

Go 最低版本为 1.25.0。默认数据目录从 `./data` 调整为 `./output/data`，代码、交易日和股本缓存使用 `./output/data/database`；分笔 CSV 示例输出到 `./output/trade`。

旧缓存不会自动搬迁。需要保留现有代码/交易日/股本缓存时，关闭所有使用它们的进程后，将 `codes.db`、`workday.db`、`gbbq.db` 及对应 SQLite WAL/SHM 文件完整复制到新目录，保留原文件备份；这些缓存表结构不变。也可以通过 `WithCodesDB/WithWorkdayDB/WithGbbqDB` 或对应 `DialDB` 选项显式配置路径。这类元数据缓存迁移与上面的旧 K 线数据库格式转换不同。
