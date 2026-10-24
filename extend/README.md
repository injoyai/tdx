# 扩展功能与包职责

新代码直接导入功能所属子包，避免因使用本地文件或导出工具引入完整网络服务依赖。

| 包 | 职责 | 主要入口 |
|---|---|---|
| `extend/local` | 通达信 `.day/.lc1/.lc5` 本地文件编解码 | `ReadDay`、`ReadMinute1/5`、`WriteDay`、`WriteMinute1/5` |
| `extend/export` | 标准行情 K 线与分笔 CSV 导出 | `KlinesToCSV`、`TradesToCSV` |
| `extend/pulltrade` | 按交易日下载历史分笔并导出各周期 CSV | `NewPullTrade`、`PullTrade.Pull/PullYear` |
| `extend/spider` | 新浪、同花顺数据源及其比例复权因子 | `GetSinaFactor*`、`GetTHSDayKline*`、`Factor/Factors` |
| `extend/httpserver` | 行情 HTTP 服务，以及独立代码/股本 HTTP 服务 | `New`、`ListenCodesHTTP`、`ListenCodesAndGbbqHTTP` |
| `extend/metadatahttp` | 从 HTTP 服务定时刷新代码表和股本缓存 | `DialCodesHTTP`、`DialGbbqHTTP` |
| `extend/codes`、`extend/gbbq`、`extend/workday` | 代码、股本与交易日历缓存 | 各包 `New` |
| `extend/manage` | 组合缓存、连接池与工作日任务 | `New` |
| `extend/pull`、`extend/pull/market` | 多市场 K 线拉取、存储与市场适配 | `pull.New`；空导入 `market` 注册内置市场 |

根包 `extend` 保留 `DoIncomes` 收益计算；上述迁出的公开入口在 `compat.go` 中保留兼容函数、类型别名和常量，并标记 Deprecated。原调用仍可编译，迁移时替换 import 和包限定名即可，参数和返回值不变。

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

本地读写和 CSV 的价格、成交量、时间及占位记录规则保持不变。CSV 仍带 UTF-8 BOM；本地 `Write*` 只返回字节，不写文件。`spider.Factor` 是第三方比例因子，与 `protocol.Factor` 的仿射复权模型不同，不能互换。

`metadatahttp` 返回的缓存客户端使用后须 `Close()`，关闭时停止定时刷新并取消在途请求。独立元数据服务返回原始 JSON，与行情服务 `New` 的 `code/msg/data` 包装不同，路由与返回结构见 [HTTP 服务说明](httpserver/README.md)。

通用基础设施继续放在 `lib/zip`、`lib/xorms`、`lib/update`；仅仓库内部复用的实现放在 `internal`。只有一个功能使用的私有辅助函数留在该功能包，不集中到通用 `utils` 包。具体迁移对应关系见 [迁移指南](../docs/v2-migration.md#扩展功能按职责拆包)。
