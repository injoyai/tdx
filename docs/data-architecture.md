# 数据同步、存储与查询边界

完整源码分组见[目录说明](structure.md)。

## 职责与依赖

- `extend/model`：共享证券、原始行情、股本事件和快照模型，不包含 ORM 标签、数据库身份或文件路径。已有协议业务类型在需要处通过别名复用，不创建同一业务对象的第二份结构。
- `extend/storage`：按数据种类定义接口和原子提交契约；不绑定 SQL、XORM 或具体数据库。
- `extend/storage/sqlite`：实现全部接口；一个 `sqlite.New(dir)` 后端可同时用于行情、代码、股本和日历。SQLite 引擎、表模型、文件路径和缓存仅属于这个包。
- `extend/pull`：证券代码、股本事件、历史交易日期及行情的同步入口，负责完整性检查、重试、取消检查和写入；不导入 metadata 查询包或 XORM。
- `extend/metadata`：集中提供 Codes、Gbbq、Workday 三个独立查询对象，接收内存数据并提供查询／计算；不联网、不读写数据库、不启动定时任务、不需要 Close。
- `extend/metadata/httpclient`：可选 HTTP 数据获取与刷新；依赖内存查询实现，自行管理请求、定时任务和 Close，不由 metadata 父包引入。
- 应用：创建客户端／连接池、选择数据集、安排任务、发布新的查询快照、关闭资源。原 `extend/manage` 已删除。

行情接口直接使用 model.Code、model.KlineDay、model.KlineMinute 等类型，Config.Store 直接接收 storage.BarStore；pull 和 SQLite 不再重导出这些类型。model.ValidateSourceTime、ValidateTimeRange、ValidateDays、ValidateMinutes 统一源时间校验，服务入口和存储入口均保留校验，不进行时区或时间戳转换。

`tdx.Client` 继续负责协议调用。没有新增通用任务引擎、全局管理器、数据库子模块或生产依赖。

## 数据流

1. 应用构造存储和客户端／连接池，再构造 pull 服务。pull.New 不同步数据或启动后台任务。
2. 应用显式调用 SyncCodes、SyncGbbq、SyncWorkdays 或 UpdateContext。
3. 同步器完整取得并校验本次范围的数据后，调用对应存储接口。
4. 存储原子提交数据及其刷新／覆盖状态。失败不得留下部分批次或成功标记。
5. 应用可从存储读取已提交快照，构造新的内存查询对象，再整体替换当前对象。

元数据同步返回的快照只有在 error 为 nil 时才表示已提交；失败返回的中间结果不可发布。若提交成功后进程在发布内存快照前退出，下次启动可直接从数据库恢复。刷新失败保留已有数据库数据和内存快照。

## 接入复杂度

使用内置 SQLite 不需要实现任何接口：用同一个后端对象分别填入 Config.Store、CodesStore、GbbqStore、WorkdayStore；只使用哪种功能就传哪种接口。只同步元数据时无需行情 Store，也无需导入 market 注册行情适配器。

自定义后端按需实现：

| 接口 | 方法 | 语义 |
| --- | --- | --- |
| storage.CodesStore | LoadCodes、ReplaceCodes | 按交易所读取／原子替换完整快照和刷新时间 |
| storage.GbbqStore | LoadGbbq、ReplaceGbbq | 按证券读取／原子替换完整事件历史和刷新时间 |
| storage.WorkdayStore | LoadWorkdays、SaveWorkdays | 读取日历／合并相容来源覆盖区间，原子修正本次范围 |
| storage.BarStore | 行情查询、增量写入、覆盖和任务状态 | 保留源时间、删尾重插、历史补全、失败恢复 |

方法名称区分数据类型，一个对象可以实现多个小接口，无须额外包一层对象。元数据接口使用 context；行情接口沿用源时间查询契约，UpdateContext 在代码和重试边界检查取消。原生连接池支持取消等待；自定义连接池可提供 DoContext，否则须自行保证借用等待有界。底层 Client 必须有有限超时：取消不会强行关闭从共享池借出的客户端，已开始的单次请求在返回后检查取消。

## 范围、事务与来源

CodeSnapshot.Exchanges 是明确的标准市场范围，当前 SyncCodes 只支持沪深北，默认三者全部；不能用一次沪市响应删除深市记录。GbbqSnapshot.Codes 是明确证券范围；同步当前股票不会删除未请求的退市证券事件或历史行情。

FetchedAt 是同步完成时间，不是行情时间。数据与刷新时间在同一个事务中写入，应用不再分别保存元数据和完成标记。行情的分钟数据与覆盖状态仍在同一个事务中提交。不同数据集之间不要求一个大事务，也没有跨进程刷新锁；多个写入者须由应用协调。

同一个后端命名空间表示同一份来源数据，当前同步来源是 TDX。其他来源应使用独立数据集，不能在同一范围中混存不同来源的快照。元数据完整空响应可清除明确范围内的数据；请求失败、条数不符、非法记录不可视为空快照。行情空增量继续跳过写入，避免侵蚀历史。

## 原始行情与计算结果

原始日线只包含源时间、OHLC、成交量和成交额，不再保存 FloatStock、TotalStock、Turnover。历史股本、换手率、复权因子和复权价格从独立事件与行情计算。旧 SQLite 日线的额外列不会自动删除；新读写不映射这些派生列。

metadata.NewGbbq 复制并排序事件；GetEquity 按历史生效日期查找，不用当前股本填历史。Turnover 返回 (value, available)，缺失／无效股本与真实零换手率不同。严格重现“当时已知”的事件还需要来源公告时间与版本信息，本次没有新增这类数据。

代码表只表示当前快照，不能据此宣称历史证券池完整或回测消除了幸存者偏差。

## 日历

SyncWorkdays 当前从 sh000001 已返回的日线提取中国市场历史日期，保留明确的市场、来源和源日期覆盖范围。空响应返回错误，不写入覆盖状态，也不可发布为新的日历。现有接口不提供港美股／期货日历同步，也不预测未来节假日。

Workday.Status 返回 (observedTradingDay, coveredBySource)。覆盖内未出现仅表示该来源未观察到交易，不能证明官方休市；覆盖外返回未知。Is 和 Range 是仅遍历已观察日期的便捷入口。不同来源或不相接的覆盖范围不能合并成一段虚假的连续覆盖。

## 生命周期与兼容边界

查询对象不持有外部资源；构造器复制输入，读取结果不暴露可修改的内部记录。metadata.Codes.Update 仍可原子替换内存快照，供显式 HTTP 客户端复用；标准连接只维护自身的 Decimal 缓存。

Service.Close 只拒绝后续操作，不关闭注入的后端或连接池。应用应先等待任务退出，再关闭服务和共享资源。sqlite.New 创建的连接由后端 Close；NewMetadata(db)、NewUpdate(db) 借用的引擎由调用方关闭。HTTP 元数据服务接收现成查询对象，不再构造自动刷新缓存。extend/metadata/httpclient 是独立的显式 HTTP 连接器，调用 Dial 会访问网络，其定时任务和 Close 契约保留。HTTP 股本缓存校验分组与事件代码一致，通过 metadata.NewGbbq 构造快照后再原子替换；查询复用同一业务实现，失败不覆盖旧快照。

行情 SQLite source-time-v1 目录和原始量价口径保留。新合并后端将元数据写到 metadata.db；旧独立元数据库不会自动搬迁，旧 workday 表也不会被自动重写成带市场和覆盖信息的新日历表。迁移前保留原文件，显式同步或转换到新数据集。
