# SQLite 后端

本包是主模块中的普通包，集中维护全部 XORM 与 SQLite 实现，没有独立 go.mod。不内置 MySQL；其他后端由应用实现 storage 接口。

| 入口 | 作用 |
| --- | --- |
| New(dir) | 返回同时实现元数据与行情接口的后端 |
| NewPull(dir, maxCachedEngines) | 仅创建行情后端 |
| Open(filename) | 创建父目录，打开 SameMapper、单连接的原生引擎 |
| NewMetadata(db) | 在外部 SQLite 引擎上实现三个元数据接口，不接管引擎 |
| NewUpdate(db, prefix...) | 实现更新标记接口，不接管引擎 |

合并后端的元数据位于 metadata.db；行情继续按证券／年份分文件，updated.db 保存任务状态，minute-scans.db 保存空扫描记录，默认最多缓存 32 个空闲行情引擎。NewPull 可调整缓存上限，使用中引擎不淘汰。

新日线只有 Time、OHLC、Volume、Amount；旧库额外列不自动删除。分钟行情与覆盖状态、元数据与刷新状态均原子提交。部分批次、刷新状态或取消失败不能留下半次提交。代码快照仅替换明确交易所，股本快照仅替换明确证券。日历区分市场和来源，保存时修正扫描范围并合并相容覆盖区间。

先停止使用者，再关闭后端；New 创建的连接由后端 Close，Open 创建及注入 NewMetadata/NewUpdate 的引擎由应用 Close。所有构造器不联网、不启动后台任务。

旧元数据文件不自动搬迁；旧日历不自动推断覆盖范围。详细接口和迁移边界见[存储契约](../../../docs/storage.md)、[架构](../../../docs/data-architecture.md)和[迁移指南](../../../docs/v2-migration.md)。

验证：`go test -race ./storage/sqlite/...`。测试仅使用临时本地数据库。
