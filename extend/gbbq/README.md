# 股本缓存与复权查询

`github.com/injoyai/tdx/v2/extend/gbbq` 缓存标准证券的股本、除权除息记录，提供 `Interface` 查询接口及 `Gbbq` 持久化实现。

```go
import "github.com/injoyai/tdx/v2/extend/gbbq"

cache, err := gbbq.New(gbbq.WithClient(c)) // c 为 tdx.Client
if err != nil {
    return err
}
defer cache.Close()
ks, err := cache.QFQKlineDay("sh600519")
```

`New` 默认建连、打开 `./output/data/database/gbbq.db` 并立即刷新，再启动定时任务，因此可能返回网络或数据库错误。`DefaultSpec` 为 `0 5 9 * * *`，按固定东八区每天 09:05 更新。已有数据库无需搬迁，表结构和字段保持不变。

- `WithClient` / `WithDB` 直接注入资源，由调用方关闭。
- `WithDialClient` / `WithDialDB` 工厂创建的资源归缓存所有，初始化失败时也会清理；数据库工厂类型为 `xorms.DialFunc`。
- `WithSpec` / `WithRetry` 配置更新计划与重试次数，`WithOptions` 组合选项。
- `Update()` 串行刷新，完整拉取后在事务中替换数据库记录；失败不发布部分新结果。
- `Close()` 可重复或并发调用，等待在途更新并停止调度，释放自建资源；关闭后 `Update()` 返回 `net.ErrClosed`，已加载数据仍可查询。

`GetEquity`、`GetTurnover`、`GetXRXDs`、`GetFactors` 提供内存查询；`QFQ` / `HFQ` 接收 `protocol.Klines`，`QFQKlineDay` / `HFQKlineDay` 直接拉取并复权。迁移不改变 `protocol` 的单位和计算算法。`All()` 只复制映射，记录与切片沿用共享读取语义，调用方应视为只读。

`manage.WithGbbq` 接收 `gbbq.Interface`，`manage.WithDialGbbq` 接收 `gbbq.DialFunc`。管理器仅懒初始化自己创建的占位对象，显式传入 `&gbbq.Gbbq{}` 可禁用股本回填。`extend.GbbqHTTP` 同样实现本接口，HTTP 入口继续位于 `extend`。

`lib/gbbq` 负责离线文件下载和解密，与本包的网络记录缓存不同；文件单位未独立核实，不能按同名字段直接换算。旧根包入口对应关系见 [迁移指南](../../docs/v2-migration.md#股本缓存与默认配置归位)。
