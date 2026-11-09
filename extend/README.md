# 扩展功能与包职责

新代码直接导入功能所属子包，避免因使用本地文件或导出工具引入完整网络服务依赖。

| 包 | 职责 | 主要入口 |
|---|---|---|
| `extend/local` | 通达信 `.day/.lc1/.lc5` 本地文件编解码 | `ReadDay`、`ReadMinute1/5`、`WriteDay`、`WriteMinute1/5` |
| `extend/export` | 标准行情 K 线与分笔 CSV 导出 | `KlinesToCSV`、`TradesToCSV` |
| `extend/spider` | 新浪、同花顺数据源及其比例复权因子 | `GetSinaFactor*`、`GetTHSDayKline*`、`Factor/Factors` |
| `extend/model` | 共享业务记录、快照与校验 | Code、CodeSnapshot、GbbqSnapshot、WorkdaySnapshot |
| `extend/metadata` | 证券、股本、历史日历的内存查询 | NewCodes、NewGbbq、NewWorkday |
| `extend/metadata/httpclient` | HTTP 元数据获取与刷新 | DialCodesHTTP、DialGbbqHTTP |
| `extend/storage`、`extend/storage/sqlite` | 存储契约与 SQLite 实现 | BarStore；sqlite.New 提供行情与元数据具体方法 |
| `extend/pull`、`extend/pull/market` | 元数据获取、行情同步与历史分笔下载 | pull.New、pull.NewTrade |
| `extend/httpserver` | 行情 HTTP 服务，以及独立代码/股本 HTTP 服务 | `New`、`ListenCodesHTTP`、`ListenCodesAndGbbqHTTP` |

扩展均位于 extend：同步入口见 [pull](pull/README.md)，内存查询见 [metadata](metadata/README.md)，持久化契约与实现见 [storage](storage/README.md)，共享数据见 model。元数据 HTTP 客户端位于 metadata/httpclient。

`pull.Service` 通过显式注入的 Store 管理日线/分钟线数据；同包的 `pull.Trade` 负责历史分笔及合成分钟线 CSV 下载导出。CSV 编码继续复用 `extend/export`，仅保留年度分笔和 1 分钟数据，其他周期逐个派生导出。

证券索引、股本计算和交易日历集中在 [metadata](metadata/README.md)，入口为 NewCodes、NewGbbq、NewWorkday；原三个扩展包已删除。

根包 `extend` 只提供 `DoIncomes` 收益计算。其余功能直接导入职责子包，不提供旧入口、转发函数或类型别名。

## 区间价格涨跌

```go
incomes, err := extend.DoIncomes(ks, startAt, 0, 1, 5)
if err != nil { return err }
for _, income := range incomes {
    rate, available := income.RiseRate()
    // rate 为比例，0.01 表示 1%；available=false 表示不可计算。
    _ = rate
    _ = available
}
```

输入 K 线须按时间升序排列，记录非 nil，收盘价为有限值；非法输入返回错误，不返回部分结果。起点按 startAt 所在的东八区日期选取第一条记录，兼容零点和 15:00 日线标签。偏移以 K 线条数计，0 为基准记录、1 为下一条，负数及越界项忽略，保留请求顺序和重复项。

Source、Current 完整复制输入 K 线，保留时间、成交量、成交额等字段。Rise 返回收盘价差；RiseRate 返回 (rate, available)，基准价为零或计算结果非有限值时不可用，真实零涨跌返回 (0, true)。String 将不可用结果显示为“不可计算”；有限比例转换为百分数溢出时显示“超出显示范围”。本工具只比较传入的收盘价，不计算手续费或现金分红。

```go
import (
    "github.com/injoyai/tdx/v2/extend/export"
    "github.com/injoyai/tdx/v2/extend/local"
)

ks, err := local.ReadDay(tdxDir, "sz000001")
if err != nil {
    return err
}
return export.KlinesToCSV("./output/export/sz000001.csv", "sz000001", "平安银行", ks)
```

本地读写和 CSV 的价格、成交量、时间及占位记录规则保持不变。CSV 的 K 线输入须为整数手：行情查询先用 `KlineResp.Klines()`，一站式复权和本地读取返回的数据可直接导出；指数原始 `StandardResponse().List` 不能直接使用。CSV 仍带 UTF-8 BOM；本地 `Write*` 只返回字节，不写文件。`spider.Factor` 是第三方比例因子，与 `protocol.Factor` 的仿射复权模型不同，不能互换。

`httpclient` 返回的缓存客户端使用后须 `Close()`，关闭时停止定时刷新并取消在途请求。独立元数据服务返回原始 JSON，与行情服务 `New` 的 `code/msg/data` 包装不同，路由与返回结构见 [HTTP 服务说明](httpserver/README.md)。

HTTP 股本事件须携带有效代码与日期，并与所在代码分组一致。客户端复用 metadata.NewGbbq 的校验和排序，只在整份响应有效时替换快照；失败保留原查询结果。

通用基础设施继续放在 `lib/zip`、`lib/update`；仅仓库内部复用的实现放在 `internal`。只有一个功能使用的私有辅助函数留在该功能包，不集中到通用 `utils` 包。具体迁移对应关系见 [迁移指南](../docs/v2-migration.md#扩展功能按职责拆包)。

`spider.GetTHSDayKline` 及其 Full/FactorFull 组合接口统一返回整数手成交量。同花顺数量必须是可表示的非负整数股数且能整除 100，否则报错，不再把零股静默四舍五入；Full 的不复权数据也通过 `KlineResp.Klines()` 适配。

SQLite 连接与行情存储位于主模块的 storage/sqlite 单独包，见 [存储说明](../docs/storage.md)。元数据通过 pull.Fetch* 获取，查询包只接收记录，应用自行选择持久化方式；其他行情后端由调用方实现 storage.BarStore 并注入 pull；本库不提供 MySQL 实现。
