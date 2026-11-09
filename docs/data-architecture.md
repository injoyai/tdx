# 数据同步、存储与查询边界

完整源码分组见[目录说明](structure.md)。

## 职责与依赖

- `extend/model`：共享证券、原始行情、股本事件和快照模型，不包含 ORM 标签、数据库身份或文件路径。已有协议业务类型在需要处通过别名复用，不创建同一业务对象的第二份结构。
- `extend/storage`：仅定义行情 BarStore 和原子提交契约；不绑定 SQL、XORM 或具体数据库。
- `extend/storage/sqlite`：实现 BarStore 并提供元数据具体读写方法；一个 `sqlite.New(dir)` 后端可同时用于行情、代码、股本和日历。SQLite 引擎、表模型、文件路径和缓存仅属于这个包。
- `extend/pull`：获取证券代码、股本事件、历史交易日期快照，并同步行情；负责完整性检查、重试和取消，只有行情同步调用存储；不导入 metadata 查询包或 XORM。
- `extend/metadata`：集中提供 Codes、Gbbq、Workday 三个独立查询对象，接收内存数据并提供查询／计算；不联网、不读写数据库、不启动定时任务、不需要 Close。
- `extend/metadata/httpclient`：可选 HTTP 数据获取与刷新；依赖内存查询实现，自行管理请求、定时任务和 Close，不由 metadata 父包引入。
- 应用：创建客户端／连接池、选择数据集、安排任务、发布新的查询快照、关闭资源。原 `extend/manage` 已删除。

行情接口直接使用 model.Code、model.KlineDay、model.KlineMinute 等类型，Config.Store 直接接收 storage.BarStore；pull 和 SQLite 不再重导出这些类型。model.ValidateSourceTime、ValidateTimeRange、ValidateDays、ValidateMinutes 统一源时间校验，服务入口和存储入口均保留校验，不进行时区或时间戳转换。

`tdx.Client` 继续负责协议调用。没有新增通用任务引擎、全局管理器、数据库子模块或生产依赖。

## 数据流

1. 应用构造客户端／连接池，再构造 pull 服务；行情同步另需存储。pull.New 不执行请求或启动后台任务。
2. 应用显式调用 FetchCodes、FetchGbbq、FetchWorkdays，成功获取并完整校验后返回快照；失败返回零值，不暴露部分数据。
3. 应用可直接构造内存查询对象并整体发布；需要持久化时，由应用先显式保存快照，保存成功后再发布。
4. SQLite 元数据方法原子提交数据及其刷新／覆盖状态。失败不得留下部分批次或成功标记；保存成功但尚未发布即退出时，下次可从数据库恢复。
5. 行情仍由 UpdateContext 通过 BarStore 执行增量同步与状态提交。

元数据 Fetch 成功仅表示获取完成，不表示已保存。获取或保存失败时，应用继续保留旧查询对象；不会因调用 Fetch 自动覆盖数据库。

## 接入复杂度

仅获取元数据时，传入 Config.Pool 即可，无需实现存储接口或注册行情市场。Fetch 返回值可直接传给 metadata 查询构造器。需要 SQLite 持久化时直接调用后端具体方法：

| 数据 | 读取 | 显式保存 |
| --- | --- | --- |
| 代码 | LoadCodes | ReplaceCodes：按交易所原子替换快照和刷新时间 |
| 股本 | LoadGbbq | ReplaceGbbq：按证券原子替换事件历史和刷新时间 |
| 日历 | LoadWorkdays | SaveWorkdays：合并相容来源覆盖区间，修正本次范围 |

行情使用内置 SQLite 时只需 Config.Store。其他行情后端实现 storage.BarStore，包含行情查询、增量写入、分钟覆盖和任务状态；标准证券自动发现通过 ListCodes(ctx) 提供本轮列表，不要求数据库。自定义元数据保存方式由应用决定，pull 不要求额外接口。

SQLite 元数据方法使用 context；行情接口沿用源时间查询契约，UpdateContext 在代码和重试边界检查取消。原生连接池支持取消等待；自定义连接池可提供 DoContext，否则须自行保证借用等待有界。底层 Client 必须有有限超时：取消不会强行关闭从共享池借出的客户端，已开始的单次请求在返回后检查取消。

## 范围、事务与来源

CodeSnapshot.Exchanges 是明确的标准市场范围，当前 FetchCodes 只支持沪深北，默认三者全部；不能用一次沪市响应删除深市记录。GbbqSnapshot.Codes 是明确证券范围；同步当前股票不会删除未请求的退市证券事件或历史行情。

FetchedAt 是获取完成时间，不是行情时间，也不是保存成功证明。应用显式保存到 SQLite 时，数据与刷新时间在同一个事务中写入，无须再单独写元数据完成标记。行情的分钟数据与覆盖状态仍在同一个事务中提交。不同数据集之间不要求一个大事务，也没有跨进程刷新锁；多个写入者须由应用协调。

同一个后端命名空间表示同一份来源数据，当前同步来源是 TDX。其他来源应使用独立数据集，不能在同一范围中混存不同来源的快照。元数据完整空响应可清除明确范围内的数据；请求失败、条数不符、非法记录不可视为空快照。行情空增量继续跳过写入，避免侵蚀历史。

## 原始行情与计算结果

原始日线只包含源时间、OHLC、成交量和成交额，不再保存 FloatStock、TotalStock、Turnover。历史股本、换手率、复权因子和复权价格从独立事件与行情计算。旧 SQLite 日线的额外列不会自动删除；新读写不映射这些派生列。

metadata.NewGbbq 复制并排序事件；GetEquity 按历史生效日期查找，不用当前股本填历史。Turnover 返回 (value, available)，缺失／无效股本与真实零换手率不同。严格重现“当时已知”的事件还需要来源公告时间与版本信息，本次没有新增这类数据。

代码表只表示当前快照，不能据此宣称历史证券池完整或回测消除了幸存者偏差。

## 日历

FetchWorkdays 当前从 sh000001 已返回的日线提取中国市场历史日期，保留明确的市场、来源和源日期覆盖范围。空响应返回错误，不写入覆盖状态，也不可发布为新的日历。现有接口不提供港美股／期货日历同步，也不预测未来节假日。

Workday.Status 返回 (observedTradingDay, coveredBySource)。覆盖内未出现仅表示该来源未观察到交易，不能证明官方休市；覆盖外返回未知。Is 和 Range 是仅遍历已观察日期的便捷入口。不同来源或不相接的覆盖范围不能合并成一段虚假的连续覆盖。

## 生命周期与兼容边界

查询对象不持有外部资源；构造器复制输入，读取结果不暴露可修改的内部记录。metadata.Codes.Update 可由应用显式替换代码快照，HTTP 获取返回的查询对象也可以直接整体发布；标准连接只维护自身的 Decimal 缓存。

Service.Close 只拒绝后续操作，不关闭注入的后端或连接池。应用应先等待任务退出，再关闭服务和共享资源。sqlite.New 创建的连接由后端 Close；NewMetadata(db)、NewUpdate(db) 借用的引擎由调用方关闭。HTTP 元数据服务接收现成查询对象，不再构造自动刷新缓存。extend/metadata/httpclient 是独立的显式 HTTP 连接器，调用 Dial 会访问网络，其定时任务和 Close 契约保留。HTTP 股本缓存校验分组与事件代码一致，通过 metadata.NewGbbq 构造快照后再原子替换；查询复用同一业务实现，失败不覆盖旧快照。

行情 SQLite source-time-v1 目录和原始量价口径保留。新合并后端将元数据写到 metadata.db；旧独立元数据库不会自动搬迁，旧 workday 表也不会被自动重写成带市场和覆盖信息的新日历表。迁移前保留原文件，显式同步或转换到新数据集。
