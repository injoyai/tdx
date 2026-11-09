# SQLite 后端

本包是主模块中的普通包，集中维护全部 XORM 与 SQLite 实现，没有独立 go.mod。不内置 MySQL；其他行情后端由应用实现 storage.BarStore；元数据持久化由应用自行安排。

| 入口 | 作用 |
| --- | --- |
| New(dir) | 返回提供行情与元数据读写方法的后端 |
| NewPull(dir, maxCachedEngines) | 仅创建行情后端 |
| Open(filename) | 创建父目录，打开 SameMapper、单连接的原生引擎 |
| NewMetadata(db) | 在外部 SQLite 引擎上提供元数据读写方法，不接管引擎 |

元数据通过应用显式调用 ReplaceCodes、ReplaceGbbq、SaveWorkdays 保存，pull.Fetch* 不调用这些方法。合并后端的元数据位于 metadata.db；行情继续按证券／年份分文件，updated.db 保存任务状态，minute-scans.db 保存空扫描记录，默认最多缓存 32 个空闲行情引擎。NewPull 可调整缓存上限，使用中引擎不淘汰。

新日线只有 Time、OHLC、Volume、Amount；旧库额外列不自动删除。分钟行情与覆盖状态、元数据与刷新状态均原子提交。部分批次、刷新状态或取消失败不能留下半次提交。代码快照仅替换明确交易所，股本快照仅替换明确证券。日历区分市场和来源，保存时修正扫描范围并合并相容覆盖区间。

Amount 为可空金额：nil 写入原列的 SQL NULL，非 nil 的 0 保持真实零。已有 Amount 列继续使用，历史零值不自动改成 NULL，旧额外状态列不映射、不删除。不会批量改写既有行情数据。

先停止使用者，再关闭后端；New 创建的连接由后端 Close，Open 创建及注入 NewMetadata 的引擎由应用 Close。所有构造器不联网、不启动后台任务。

旧元数据文件不自动搬迁；旧日历不自动推断覆盖范围。详细接口和迁移边界见[存储契约](../../../docs/storage.md)、[架构](../../../docs/data-architecture.md)和[迁移指南](../../../docs/v2-migration.md)。

完成标记直接由 PullStore 实现，表模型为私有类型，不再提供 NewUpdate、UpdateStore 或 Record。updated.db 和 update 表的 Key/Time 列不变。

New／NewPull 使用解析符号链接后的绝对根目录作为 Identity，目录别名共享任务身份。旧相对路径完成键可能触发一次增量复查，旧标记保留。行情读写拒绝不安全的市场路径／文件名，避免越界和斜线替换碰撞；自定义 `A/B` 返回错误，`A-B` 仍可用。期货保留规范 `cff/IF2609` 及原文件布局。分钟年份限 1..9999；非法输入的 DayFile／MinuteFile 返回空字符串。不会自动迁移或修复曾混写的旧文件，完整限制见[文件说明](../../../docs/storage.md#文件与旧库)。

验证：`go test -race ./extend/storage/sqlite/...`。测试仅使用临时本地数据库。
