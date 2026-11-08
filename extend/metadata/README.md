# 元数据内存查询

本包集中提供证券索引、历史股本与复权查询、中国市场历史日历。三个对象独立构造，按需使用；不联网、不读写数据库、不启动后台任务，无需 Close。数据获取使用 Client 或 pull，同步与持久化契约见[数据架构](../../docs/data-architecture.md)和[存储接口](../../docs/storage.md)。

需要从 HTTP 服务刷新数据时，显式导入独立子包 [httpclient](httpclient/README.md)；该子包持有请求与定时任务，需 Close。导入本包不会自动引入 HTTP 客户端。

```go
import "github.com/injoyai/tdx/v2/extend/metadata"

index := metadata.NewCodes(codeSnapshot.Rows)
name := index.GetName("sh600000")

equity, err := metadata.NewGbbq(eventSnapshot.Rows)
if err != nil { return err }
turnover, available := equity.Turnover("sh600000", at, volumeInShares)

calendar, err := metadata.NewWorkday(calendarSnapshot)
if err != nil { return err }
observed, covered := calendar.Status(at)
```

## 证券代码

Codes 提供 Get、GetName、Iter、GetStocks、GetETFs、GetIndexes 及对应代码列表查询。记录直接使用 model.CodeModel；NewCodes(nil) 创建空索引。HTTP 客户端复用该实现。

构造、Update 和读取均复制记录。Update 只替换内存快照，不触发数据同步；并发读取与更新受保护。代码输入的标准市场规范化复用 model.StandardCode。当前代码表不等于完整历史证券池。

## 股本与复权

NewGbbq 校验、复制并按时间排序事件。GetEquity 查询历史生效股本；记录中的 15:00 不影响所属日期零点生效。Turnover 返回 (value, available)，缺失／无效股本或负成交量不可计算，真实零成交量可得到有效零换手率。

GetXRXDs、GetXRXDMap、GetFactors、QFQ、HFQ 基于已有事件计算；查询结果不暴露可修改的内部记录。QFQ/HFQ 输入为 protocol.Klines（协议整数手口径），量额不复权。统一 Client 的股数结果先通过 KlineResp.Klines 转换。

lib/gbbq 独立负责原始文件下载、解密和解析，不是本包的另一份实现。文件原始单位未独立核实，不自动转换为网络股本模型。

## 历史交易日历

NewWorkday 接收 model.WorkdaySnapshot，保留市场、来源和覆盖范围；当前仅支持中国市场历史日历，不预测未来节假日。Status 区分已观察交易日期与来源覆盖：覆盖内未观察到交易不等于官方休市，覆盖外为未知。

Is、Range、RangeYear、Iter、IterYear 只查询或遍历已观察日期。输入与 Snapshot 返回的日期切片均独立复制。

原 extend/codes、extend/gbbq、extend/workday 已合并至本包，不保留旧路径或转发接口；调用方需要接口时自行声明实际使用的方法，HTTP 服务使用 httpserver.CodesSource。
