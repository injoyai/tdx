# 示例

27 个独立入口按主题分类，重复接口通过少量参数选择用法。`common/` 是查询后自动关闭客户端的辅助包，不能单独运行。

## 运行方式

使用 Go 1.25+，从项目根目录运行，生成的数据统一放在 `./output/`：

```sh
# 跨市场客户端
go run ./examples/client/UnifiedClient

# 查看参数，不连接服务器
go run ./examples/klines/GetKline -h

# 编译所有示例并执行离线测试，不执行 main
go test ./examples/...
```

普通查询结束后自动退出。HTTP 服务会监听本地端口；服务器测速包含并发探测。联网结果取决于证券、交易日和服务器保留范围，真实验证遵循 [项目规则](../PROJECT_RULES.md) 的预算与限流要求。文件导出与拉取服务可能更新同名文件或数据库。

## 示例索引

| 分类 | 示例 | 内容与使用条件 |
| --- | --- | --- |
| client | [UnifiedClient](client/UnifiedClient/main.go) | 统一客户端与跨市场日线 |
| client | [GetExHq](client/GetExHq/main.go) | 扩展连接、市场发现与港股代码查询 |
| client | [Manage](client/Manage/main.go) | 连接池、交易日历、代码缓存、深圳指数与 ETF 数量统计 |
| codes | [GetCodeAll](codes/GetCodeAll/main.go) | 市场总数、分页或完整代码列表、沪深北股票代码 |
| codes | [GetBlockData](codes/GetBlockData/main.go) | 板块成分、指数代码关联、专业板块与板块行情 |
| codes | [GetTdxHy](codes/GetTdxHy/main.go) | 通达信与申万行业归属 |
| quotes | [GetQuote](quotes/GetQuote/main.go) | 五档报价，多个证券作为位置参数传入 |
| quotes | [GetMinute](quotes/GetMinute/main.go) | 当日分时；`-date YYYYMMDD` 选择历史日期 |
| quotes | [GetCallauction](quotes/GetCallauction/main.go) | 集合竞价 |
| klines | [GetKline](klines/GetKline/main.go) | 股票或指数各周期 K 线，支持分页、全部历史和按日期停止 |
| klines | [MergeKlines](klines/MergeKlines/main.go) | 241 根分钟线合并为 5/15/60 分钟；`-compare` 对照普通分钟线开盘记录 |
| trades | [GetTrade](trades/GetTrade/main.go) | 实时或历史分笔，分页或整日查询 |
| trades | [TradesToKlines](trades/TradesToKlines/main.go) | 历史分笔合成分钟线及 5/15/60 分钟线，需指定 `-date` |
| finance | [GetFinanceInfo](finance/GetFinanceInfo/main.go) | 财务信息与 F10 公司资料 |
| finance | [GetGbbq](finance/GetGbbq/main.go) | 原始股本变迁与除权除息记录 |
| finance | [GetQFQKline](finance/GetQFQKline/main.go) | 一份日线计算前后复权、复权因子、股本与换手率 |
| finance | [GetKlineDayFactor](finance/GetKlineDayFactor/main.go) | 同花顺复权因子对照，需访问外部数据源 |
| finance | [Income](finance/Income/main.go) | 已落库日线的收益计算，先用 `PullV2` 准备 `sz000001` 日线 |
| data | [DumpReportFile](data/DumpReportFile/main.go) | 报表下载、解压与解析，写入 `./output/dump/` |
| data | [DecodeGBBQ](data/DecodeGBBQ/main.go) | 离线解析股本文件，`-file` 指定已解压文件、`-code` 指定裸代码 |
| data | [FetchLC1ForTest](data/FetchLC1ForTest/main.go) | 拉取并导出 `.lc1/.lc5`，默认写入 `./output/lc1/` |
| data | [ReadLocalDelisted](data/ReadLocalDelisted/main.go) | 离线读取退市股缓存，客户端安装目录作为位置参数 |
| data | [PullV2](data/PullV2/main.go) | 日线/分钟线拉取与落库查询，默认单证券、最近一个月、串行执行 |
| data | [PullTrade](data/PullTrade/main.go) | 按年拉取标准股票/ETF 分笔，支持 `-year`、`-code`，写入 `./output/trade/` |
| services | [CodesHTTP](services/CodesHTTP/main.go) | 代码缓存 HTTP 服务与对应客户端，端口 10033 |
| services | [HTTPServer](services/HTTPServer/main.go) | 完整行情 HTTP API，见 [httpserver 文档](../extend/httpserver/README.md) |
| tools | [FastHosts](tools/FastHosts/main.go) | 使用 `Addrs.Sort` 测速排序，不作为默认真实行情验证命令 |

## 合并入口的常用参数

```sh
# 北交所完整代码；沪深北股票代码
go run ./examples/codes/GetCodeAll -exchange 2 -all
go run ./examples/codes/GetCodeAll -stocks

# 指数日线、1 分钟线、完整 30 分钟线、年线、按日期停止
go run ./examples/klines/GetKline -code sh000001
go run ./examples/klines/GetKline -type 7 -count 20
go run ./examples/klines/GetKline -type 2 -all
go run ./examples/klines/GetKline -type 11
go run ./examples/klines/GetKline -until 20261001

# 历史分时、历史整日分笔与分钟合成
go run ./examples/quotes/GetMinute -date 20261012
go run ./examples/trades/GetTrade -date 20261012 -all
go run ./examples/trades/TradesToKlines -date 20261012 -code sh600000

# 指定证券，只拉日线；共用 ./output/pullv2/
go run ./examples/data/PullV2 -minute=false sz000001 sh000001 hk00700 usAAPL

# 全市场与起始日期需显式选择，运行前确认范围
go run ./examples/data/PullV2 -all -since 20260101

# 离线读取通达信缓存
go run ./examples/data/ReadLocalDelisted /path/to/通达信
```

`GetKline` 的 `-type` 使用 `protocol.TypeKline*` 数值，默认 9（日线）。`-until` 向前翻页并保留首次早于指定日期的停止记录，不能与 `-all` 同用。241 根分钟线与分笔合成仅适用于标准股票/ETF。

`PullV2` 的日线和分钟线至少开启一种，正常执行使用增量去重，`-force` 可忽略当日完成标记。配置与存储单位见 [pull 文档](../extend/pull/README.md)。

## 清理与合并

| 原示例 | 当前入口 |
| --- | --- |
| GetCount、GetCode、GetStockCodeAll、GetBjCodes | codes/GetCodeAll |
| Codes2、GetETFCodeNumber、WorkdayAndCodes | client/Manage |
| GetBlockWithIndex、GetSpBlock | codes/GetBlockData |
| GetKlineDay、GetIndex、GetKlineDayAll、GetKlineMinute、GetKline30MinuteAll、GetKlineUntil、GetKlineYear | klines/GetKline |
| GetKlineMinute241 | klines/MergeKlines |
| GetHistoryMinute | quotes/GetMinute |
| GetHistoryTrade、GetHistoryTradeAll | trades/GetTrade |
| GetFQ、GetTurnover | finance/GetQFQKline |
| PullKline | data/PullV2 |

删除了硬编码 IP 列表的 `Connect`、重复连接扫描的 `CheckHosts`、无效地址演示的 `DialHostsRange` 和全市场临时排查脚本 `CheckGetHistoryErr`。其余用法见上表，旧目录命令需改用当前路径和参数。
