# 扩展公开契约

本文件与 `docs/storage.md` 是发布和适配验收依据。固定的是可观察行为及方法边界；不固定私有表、缓存、任务键哈希实现或数据源实时覆盖量。后续修改公开字段、方法、单位、空值、替换范围、回调频率或磁盘布局时，须说明源码、数据及行为兼容影响，同步迁移说明和契约测试。破坏既有契约的变更须明确标注，不能作为无行为变化的内部调整发布。

## model

- `Code.Market/Code.Code` 共同定义证券身份；市场路由与输入形式见 `extend/pull/README.md`。代码主体保留期货大小写，不把不同身份混为一条记录。
- `KlineDay.Time` 为 `YYYY-MM-DD`；`KlineMinute.Time` 为 `YYYY-MM-DD HH:MM`。它们是源日期／钟面时间，不推导来源时区、Unix 时间或夜盘交易日。
- OHLC 和 Amount 是原币种／报价单位；Volume 使用统一 Client 的最小单位，股票为股、期货为张。`Amount=nil` 表示未知，非 nil 的零表示真实零；不能以当前值回填历史缺失金额。
- `KlineMinute.Source=""` 为原生分钟，`"trade"` 为分笔合成；合成结果不等于原生分钟精度。
- `MinuteCoverage.From` 是已处理请求范围左端；原生扫描 `Through` 为空，分笔覆盖 `Through` 为右开端点。覆盖只证明已处理服务器可提供的范围，不证明历史无缺笔。
- `MinuteEmptyScan.CheckedAt` 是缓存检查 Unix 秒；空扫描不是永久完成证明。
- `CodeSnapshot.Exchanges`、`GbbqSnapshot.Codes` 定义显式替换范围。有效空 Rows 表示该范围内没有记录；`FetchedAt` 表示获取完成时间，不是行情源时间。
- `WorkdaySnapshot` 是按市场、来源和闭区间描述的已观察日期，不是未来节假日预测。

## BarStore

完整规则见[存储契约](storage.md#行情契约)，可执行验收见[storagetest](../extend/storage/storagetest/README.md)。接口不要求 SQL、ORM、驱动或自有调度器。

- 所有方法支持并发，按证券及分钟年份隔离；查询按源时间升序、包含两端，空端点不限范围。无记录返回空集合，最后时间为空字符串，缺少覆盖／空扫描返回 nil；读取错误必须返回。
- `SaveDay/SaveMinute` 在一次原子操作内删除 `Time>=from` 的尾部后插入；分钟仅影响给定年，空 `from` 为全部日线／指定年分钟。空批次不删除、也不写覆盖状态。
- 分钟行情和非 nil 覆盖状态原子提交；失败保留原数据和原覆盖。写入边界与覆盖左端可不同，不得因认证较早的请求范围而删除服务器未返回的本地前缀。
- 相容原生覆盖取范围并集；分笔覆盖只合并相接或重叠区间，不跨过未扫描空隙。不相容覆盖按新范围替换。
- `LoadUpdate` 缺少键返回 `has=false`，只读不创建占位；`SaveUpdate` 支持首次保存和覆盖，存 Unix 秒。任务完成键由 pull 生成，Store 不解释调度窗口。
- `Identity` 非空、稳定、区分数据集且不含凭证；同一数据集的连接使用相同身份。Store 由应用关闭。
- 事务边界为单次保存，不保证多个年份、多个证券或行情与市场完成标记整体回滚，也不提供跨进程任务锁。

## Fetch 与 Unit

`Service.FetchCodes/FetchGbbq/FetchWorkdays` 仅获取并校验快照，不持久化、不自动发布、不关闭注入的资源；应用明确决定保存或使用。任一错误返回零快照，不能把部分返回当作完整替换依据。支持 context 取消及现有瞬时错误重试。

`FetchCodes` 默认沪深北，可明确指定范围；`FetchGbbq` 必须有非空且有效的标准证券范围，返回这些证券的完整事件历史；`FetchWorkdays` 使用上证指数已观察日线生成日历，空返回不能认证覆盖，须报错。

`Unit.FetchDay/FetchMinute` 的职责不同：它们执行获取和增量保存，只返回 error，方法集保持不变。可选 `ResultUnit` 增加 `FetchDayResult/FetchMinuteResult`，在配置 OnEvent 时返回 `FetchResult,error`；其数据行为必须与原方法一致。未配置 OnEvent 时仍调用旧方法。包装器若嵌入已有 ResultUnit 并覆盖旧 Fetch 方法，启用事件时也须覆盖对应 Result 方法，避免继承不匹配的行为。内置 Unit 均提供结果。第三方旧 Unit 无需改动，其事件中 `Result.Known=false`，零计数不能解释为无数据。

## 更新与事件

`Update()` 和 `UpdateContext(ctx)` 保留原调度和去重；`Update(true)` 只允许提前执行。新增：

```go
err := service.UpdateWithOptions(ctx, pull.UpdateOptions{
    AllowEarly: false,
    Recheck:    true,
})
```

`AllowEarly` 与 `Recheck` 独立：前者允许收盘边界前执行并只认证前一窗口，后者忽略本窗口完成标记。零选项等价于普通更新。复查仍读取状态并传播存储错误，保留增量边界、历史分钟覆盖和空扫描缓存，不等于重扫全部历史。成功只认证本轮开始选定的窗口；失败不写新标记，既有成功标记保留。复查失败后如需再试，继续使用 Recheck。

同一 Service 的各更新入口共用串行门控，排队可取消；不关闭共享资源。

`OnProgress` 保持每证券一次终态通知，Completed 包含最终失败证券。新增 `Config.OnEvent` 接收阶段及市场终态，两类回调在同一 Service 更新中串行执行，须及时返回，不能同步重入任一更新方法。事件不表示开始或逐页进度；并发证券之间没有固定事件顺序。

| 字段／状态 | 口径 |
| --- | --- |
| `Stage="day"/"minute"` | 单证券日线／分钟阶段终态，先于该证券 OnProgress |
| `Stage=""` | 市场终态，Code 和计数字段为零；完成标记提交失败也会产生 failed |
| `Completed/Total` | 阶段事件中为该市场已结束证券数／总数；阶段不增加计数 |
| `Attempts` | 阶段实际调用次数，包含重试；日线失败阻止分钟时为零 |
| `Result.Known` | 是否提供计数；旧 Unit 为 false |
| `Result.Fetched` | 最后一次尝试的主行情扫描向保存流程返回的记录数，保存范围过滤之前；不统计协议分页流量和兜底分笔／合成数据 |
| `Result.Saved` | 最后一次尝试中已成功保存调用的行数，含覆盖和分笔合成，不是净新增行数；阶段失败时可能已有其他年份成功提交 |
| `Result.LatestTime` | 本次主行情扫描返回的最新源时间；未返回则为空，不是库中最新时间 |
| `skipped` | Reason 明确：before_close、completed、empty_universe、covered、future_start、day_failed |
| `no_data` | 指标已知，阶段无错误、无跳过，Fetched 和 Saved 均为零；不表示永久无数据 |
| `success` | 阶段无错误且不属于上述状态；旧 Unit 成功但计数未知也属此状态。市场 success 表示本轮完成标记已保存 |
| `failed` | 获取、保存、取消或完成标记错误，通过 Err 返回；失败阶段优先于无数据判断 |

## 本次兼容影响

- `BarStore`、`Unit`、`Progress` 和旧更新方法的签名保持不变。新增更新入口、可选结果接口及 `Config.OnEvent`；Config 请用带字段名的结构字面量，无字段名构造须调整。
- SQLite 改用 `source-time-v2`，为解决大小写不敏感文件系统的证券碰撞。**不兼容旧数据目录**：旧 `source-time-v1`、Unix 格式或未标记的旧库拒绝打开，不迁移、覆盖或清理，须使用新目录。字段、源时间和量价语义不变。
- 路径各段只保留小写 ASCII 字母、数字、`-_.`，其余 UTF-8 字节编码成 `~hh`，包括大写和 `~` 本身。期货的规范分隔符先转换为 `-`。例如 `cff/IF2609` 使用 `cff-~49~462609`，与 `cff/if2609` 隔离；自定义市场名同样编码，避免 Unicode 规范化碰撞。
