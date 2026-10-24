# MEMORY.md — 项目记忆文档

> 本文件按 AGENTS.md 第 6 节要求维护，供本仓库开发协作使用，记录长期约定、决策与踩坑。使用本库的用户及其 agent 应从 README.md、docs/unified-client.md 和 examples/README.md 入手；迁移旧代码另看 docs/v2-migration.md。当前源码与正式接口文档优先于历史记录。

## 项目概况

- 通达信(tdx)协议 Go 实现：标准行情(7709端口) + 盘后数据(report file) + 扩展行情(exhq)。
- 支持沪深北交易所、K线/分时/成交、板块、财务、复权、行业归属、报表配置等。
- **输出数据约定**：所有生成/落盘的数据文件统一写入 `./output/`（已入 `.gitignore`），按场景建子目录；禁止散落写入项目根目录/`examples/` 内或其他任意路径（见 PROJECT_RULES.md 第 8.2 节）。

## 技术栈

- Go 1.25+，依赖 `github.com/injoyai/conv`(类型转换)、`github.com/injoyai/logs`(日志)。
- 参考开源：gotdx / mootdx / pytdx / tdx2db / nieqx-tdx。

## 架构

- `protocol/`：协议帧编解码与数据结构（`model_*.go`、`unit.go`、`frame.go`、`types.go`）。
- `conn.go`：内部传输；`conn_st.go` / `conn_ex.go`：协议业务连接；`query.go` / `conn_query.go`：共享业务算法及校验。
- `extend/`：离线数据读取(`local.go`)、行情拉取、爬虫、HTTP server；指标统一由 `protocol.Klines` 提供。
- `lib/`：gbbq、xorms、zip 工具。
- `examples/`：少量独立的典型用法，入口见 `examples/README.md`。

## 关键约定与决策记录

1. **北交所市场编码**：`ExchangeBJ=2`（SZ=0/SH=1/BJ=2，通达信官方文档 + gotdx 交叉验证）。
2. **量化枚举齐全**：`protocol/types.go` 已按通达信量化官方 Dict(https://help.tdx.com.cn/quant/docs/markdown/Dict.html) 补全市场类型(ExchangeNQ=44新三板/SHO=8/SZO=9/HK=31/US=74等)、OrderType、PriceType、Status、DividendType、Period 枚举。Exchange 类型是 uint8，注意 `ExchangeINE` 与 `ExchangeSHF` 同为 30。
3. **标准行情协议不支持 BJ 代码列表请求**：原始标准协议 `MCount/MCode(market=2)` 超时；统一 `GetCount/GetCode(ExchangeBJ)` 使用下述报表来源；但行情/K线正常。
4. **北交所代码获取**：`GetCodeAll(ExchangeBJ)` 走 `GetZHBFiles()` → `tdxbjmore.cfg`（含名称，可能含已分配未上市代码）；不再使用北交所官网爬虫。代码数量随报表更新，不作为固定契约。
5. **tdxstat.cfg**：全市场逐股统计，35字段，无名称字段。名称需另从 `tdxbjmore.cfg`/板块文件等获取。
6. **退市股票历史样本（2026-08）**：原代码在线数据可能已清退，不能假定线上服务器保留退市股历史，也不能由以下样本推断所有退市品种。部分退市品种使用新三板代码(400/420，市场44)。当时实测：标准行情7709 `GetKlineDay("600001"/"sh600001")` 返回0条、`GetQuote(sh600001)` 全零；扩展行情7727 当时的旧接口 `ExQuote/ExBars` 在 market 0/1/2/44 × (原代码/`T600001`) 全部返回全零（对照组 `sz000609` 正常）。这些是历史观测；当前公开业务接口统一为 `Get*`，原始任意市场请求使用 `SendFrame` 与 `protocol.MEx.Frame*`。客户端能显示退市股(如 `T600001`)是通过【功能→定制品种→退市品种管理】绑定"自设市场+原代码"后，其历史日线缓存于本地 `<安装目录>/T0002/ds_cache/`，文件命名 `<市场标识>#<代码>.~~~day`(实测 `100#T600001.~~~day`/`100#T600003.~~~day`，`100#` 为缓存命名空间/市场标识，`~~~day` 为日线缓存后缀)，非实时行情服务器提供、也不在 `vipdoc/lday` 下。**缓存格式与标准 `.day` 不同**：头部14字节(`[0:10]`保留 + `[10:14]` uint32 记录数)，之后每条32字节——`[0:4]`uint32日期YYYYMMDD、`[4:20]`4×float32开/高/低/收(**元**)、`[20:24]`float32成交额(元)、`[24:28]`uint32成交量(股)、`[28:32]`保留。
7. **板块指数(880xxx/881xxx)编码约定**：`protocol.IsIndex/AddPrefix` 已识别板块指数——880xxx(概念/风格/地区)、881xxx(行业)归属上海交易所(ExchangeSH)，`AddPrefix("880741")`→`sh880741`，`IsIndex("sh880741")`=true。板块指数日K线用自动识别指数的 `GetKlineDay`(指数解析, 量×100/涨跌家数)、实时行情用 `GetQuote` 均可直接获取；`GetQuote` 对指数类(含板块指数)跳过元数据价格修正，标准报价不依赖全局 `DefaultCodes` 初始化。板块指数不在 `GetCodeAll`/`DefaultCodes` 股票代码列表内。板块成分查询使用 `GetBlockData` / `GetSpBlock`。
8. **可转债(标准行情7709)**：`protocol.AddPrefix` 已识别可转债前缀——沪市(110/111/113/118)→`sh`、深市(123/125/126/127/128)→`sz`。日K线 `GetKlineDay`/分钟线直接可用，统一返回浮点价格，原始标准价格为厘。实时行情 `GetQuote` 由当前 `StConn` 使用同一连接读取所属交易所代码表，并在该连接内缓存精度元数据，不依赖 `DefaultCodes`、数据库或额外连接。价格按代码的 `Decimal` 校准（`Decimal=4` 时原始价格÷100）；缺少元数据返回错误，不猜测精度。
9. **期货(扩展行情7727)**：使用跨市场 `NewClient()` 或 `DialExHqDefault()`，扩展业务实现位于 `conn_ex.go`。合约使用显式交易所前缀，如 `cffIF2609`、`dceA2609`；不能假定连续/主力代码可用。批量行情用 `GetQuoteList(market, 3, 0, n)`（market: 47中金/60主力期货/30上期/28郑商/29大商/66广期）。日K用 `GetKlineDay(code, 0, n)` 或 `GetKline(TypeKlineDay, code, 0, n)`，业务入口按市场映射为实际 category=4；持仓、结算价等保留在 `Extended` 中，未知单位不宣称已确认。原始扩展品种协议分页 start 为全局品种序号；统一 `GetCode(exchange,start,count)` 按市场过滤后分页，不能互换偏移语义。
10. **`DecodeCode` 通用代码解析**：已泛化支持多市场——A股(6位数字自动补前缀, 行为不变)、港股(5位纯数字如 `00700`/`hk00700`)、美股(纯字母如 `AAPL`/`usBRK.B`, 最长前缀匹配避免 `SHOP` 被误拆为 `sh`+`OP`)、期货(需显式前缀如 `cffIF2609`/`dceA2609`, 裸合约如 `IF2609` 因无法确定交易所而报错提示用前缀)。另支持带点后缀格式 `000001.SZ`/`600000.SH`/`00700.HK`/`AAPL.US`/`IF2609.CFF`(后缀=交易所缩写, 大小写均可; 美股点代码 `BRK.B` 因后缀 B 非交易所而按美股代码解析)。前缀支持小写缩写/大写/中文名(如 `上海600000`)。注意: 标准行情7709的 Frame(model_quote/model_kline 等)只接受 A股 6 位定长代码；统一业务入口使用 `protocol.ParseInstrument` 解析与路由，港股/美股/期货走扩展行情7727的同名 `Get*`，不使用标准行情帧。代码能解析不代表相应市场已接入行情路由。
11. **manage.go `RangeIndexes` 修复(2026-09)**：历史 bug——曾误遍历 `GetETFs()`，已改为 `GetIndexes()`。若外部代码曾依赖错误行为（遍历出 ETF 列表），需注意语义变化。

- **v2 指标与复权契约**：指标统一由 `protocol.Klines` 实现；VWAP 输入量为手、金额为厘，pull 存储与标准行情指数日线的股数须先换算。复权使用仿射 Mul/Add 和 QFQPrice/HFQPrice，不再提供旧纯比例入口；第三方爬虫的比例模型独立保留。
- **数据库支持边界**：内置存储统一使用 SQLite；已移除 MySQL 专用构造函数和驱动注册。外部调用须使用默认构造函数或传入 SQLite 引擎，MySQL 存量数据需单独转换，见 `docs/v2-migration.md`。
- **默认缓存路径**：元数据缓存位于 `./output/data/database`。历史 `./data/database` 不自动搬迁，使用旧缓存时须显式配置 DB 或在停机后完整复制（含 SQLite WAL/SHM），见 `docs/v2-migration.md`。
- **元数据资源归属**：Codes/Gbbq/Workday 的 `Close()` 停止更新、等待在途刷新、关闭自建客户端/数据库；直接注入资源由调用方管理，工厂资源归构造对象。Manage 同样区分注入与工厂，并管理 pull 懒加载股本缓存；关闭时先停止调度、释放自建资源，再等待任务退出，以唤醒任务中阻塞的连接池获取。外部共享池及任务自身的阻塞须由调用方解除。独立可停止任务用 `StartTimer`，保留的 `NewTimer` 属进程级任务。HTTP 服务不再初始化全局 DefaultCodes。

- **请求等待依赖约束**：`base/maps/wait` 的等待注册与响应完成不能并发修改同一回调列表。内部 conn 在发送前完整注册异步回调，并由同一互斥锁保护注册/处理/清理；超时通过 `Client.SetTimeout` 配置，直接修改具体连接的 `Wait` 不调整请求计时器。

- **统一行情 API 与连接边界**：`Client` 是统一业务接口；`NewClient()` 返回 `Client` 接口，默认实现为跨市场 `*MarketClient`；标准 `Dial*` 直接返回 `*StConn`，`DialExHq*` 返回 `*ExConn`。公开 Conn、包装构造和 CloseAll 已移除；内部 conn 不实现 Client，只负责传输、请求关联和生命周期，协议业务及解码分别归 StConn/ExConn。共享 queries 绑定业务对象，复用分页及历史日期检查，不通过传输层绕回门面；标准报价精度缓存归 StConn。Close 完整关闭生命周期并停止重连。连接池的工厂、Get/Put/Do/Go 使用 Client，原始 SendFrame/状态/日志从具体连接使用；需要扩展全局语义的 HTTP/pull 先取得 ExConn。Codes/Gbbq/Workday 的客户端注入与相应工厂改为 Client 接口，保留空指针注入行为。财务、F10、除权、集合竞价、板块和报表已接入门面，仍只支持标准协议和原标准结果类型，非法市场或扩展单连接提前报错。两者六组基础查询统一为 `GetQuote/GetKline/GetMinute/GetHistoryMinute/GetTrade/GetHistoryTrade` 及统一返回；对应公开 `Ex*` 已移除，市场行情分页列表改为 `GetQuoteList`。代码查询统一为 `GetCount(exchange)`（int）、`GetCode(exchange,start uint32,count uint16)` 和 `GetCodeAll(exchange)`（`*CodeResp`），公开 `ExCount/ExInstruments` 已移除；扩展单市场数量和分页扫描全局列表，不新增持久缓存。`GetMarkets` 返回内置标准市场或服务器扩展表（跨市场门面合并），发现不等于行情路由接入。历史 HTTP 全局数量/分页与 pull 一轮发现缓存使用协议层保留语义；pull 快照入口为 `GetInstruments`。单协议对象复用业务算法但不额外建连；K线全部周期及 All/Until、完整分笔均使用统一返回类型，独立指数 K线和 ExBarsRange 已移除；区间入口 GetKlineRange 固定一分钟，原扩展区间字段在 RangeExtended。标准指标/复权通过 StandardResponse 提取原始整数价格与时间，扩展或旧响应数量溢出时报错。标准分页继续回填跨页昨收；241分钟线只用于标准股票/ETF。跨日分笔要求对应市场 TradingCalendar，Workday 是沪深日历，逐条保留交易日期，不自动补夜盘自然日。HTTP 适配保留原字段和单位。两类初始化独立同步，关闭清理晚到连接；未知数据尺度保留原始字段，不推测换算。支持范围及迁移说明见 `docs/unified-client.md` 和 `docs/v2-migration.md`。

- **连接生命周期依赖约束**：`ios` 的完整写入流程（包括写入统计）和重连初始化须共用门控；初始化在连接回调之前，故门控从 Dial 成功返回前开始，必须保持到握手完整发送后，不能在发布写入快照时提前释放。握手在已持有门控时直接写当次连接，不能再次经过普通加锁入口。对外使用稳定的 `MoreWriter` 入口，心跳只持有当次连接及其关闭信号。生命周期上下文须从首次拨号贯通到读取/重连，否则门面关闭时无法取消默认的首次重试。请求超时覆盖排队、发送、等待；发送被中断时关闭当次连接，防止半帧污染。底层关闭会立即归还读缓存，故协议分包使用每条连接独立的读取缓存；`Close` 取消生命周期并关闭传输，等待读取流程退出后释放底层资源，避免与读取初始化或重连状态重置竞争。

- **TCP 关闭依赖约束**：真实 TCP 重复 `Close` 返回错误，`ios` 遇到关闭错误会提前跳过断开回调和缓存清理；传输须用幂等关闭封装，`Origin()` 继续实现 `net.Conn`，但不保证具体类型为 `*net.TCPConn`。

## 本地数据文件解析（extend/local.go，参考 pytdx TdxDailyBarReader/TdxLCMinBarReader 官方协议）

- 目录结构：`vipdoc/<sh|sz|bj>/lday|minline|fzline`
- **`.day` 日线**：32字节/条
  - `00~03` uint32 日期(YYYYMMDD)
  - `04~19` 4×uint32 开/高/低/收(价格×100 整数)
  - `20~23` float32 成交额(元)，`24~27` uint32 成交量(股票为股、指数为手)，`28~31` 保留
- **`.lc1`(1分钟)/`fzline/*.lc5`(5分钟)**：32字节/条
  - `00~01` uint16 日期(year=num/2048+2004, month=(num%2048)/100, day=num%2048%100)
  - `02~03` uint16 当日分钟数(0点起)
  - `04~19` 4×float32 开/高/低/收(元)，`20~23` float32 额(元)，`24~27` uint32 量(股票为股、指数为手)，`28~31` 保留
- 本地文件读写约定：`protocol.Price` 单位为厘；`ReadDay/ReadMinute1/ReadMinute5` 返回及 `WriteDay/WriteMinute1/WriteMinute5` 输入的 `protocol.Kline.Volume` 为协议百股单位（手）。不能把该约定套用到所有在线结果：统一 `tdx.Kline` 价格为原币种/报价单位的 `float64`，量按响应 `VolumeUnit` 判断；标准股票/ETF K线为百股，标准指数/板块 K线为股，港股日线为百股，其他未确认项为 `unknown`。`StandardResponse()` 保留原始字段，不自动转换量单位；线上指数数据用于本地编码或 VWAP 前须先将股转换为百股。
- **成交量单位差异（指数 vs 股票）**：`.day/.lc1/.lc5` 的 `24~27` 字段，**指数(如 sh000001/sz399001/bj899050)单位是"手"，原值即手**；**股票单位是"股"**，需 ÷100 转手。`ReadDay/ReadMinute1/ReadMinute5/WriteDay/WriteMinute1/WriteMinute5` 已按 `protocol.IsIndex(c)` 区分处理（指数不÷100；写入时股票×100转股、指数原样写手）。判断时用 decodeCode 已带前缀的 c 直接 `IsIndex(c)`，勿再拼前缀。
- API：`ReadDay(dir, code)`、`ReadMinute1(dir, code)`、`ReadMinute5(dir, code)`；code 需带交易所前缀(如 `sz000001`)，本地文件名为 `sz000001.day` 格式。
- **写入**：`WriteDay(code, ks) ([]byte, error)`、`WriteMinute1(code, ks)`、`WriteMinute5(code, ks)`（均 `([]byte, error)`），与读取格式对称，**只返回通达信格式字节流、不落盘**，由调用方自由决定如何写入/使用（如自行 `os.WriteFile` 到 `./output/lc1/vipdoc/...`）。code 需带交易所前缀用于判断指数/股票。均在 `extend/local.go`。

## extend/pull v2 拉取服务（2026-08 新增）

> 通用可插拔的日线+1分钟线拉取服务，覆盖沪深股票/指数/ETF/LOF/板块/期货/港股/美股，每天增量更新。v2 统一使用本服务；旧拉取服务的存量数据库不自动转换，不能仅改目录复用，处理要求见 `docs/v2-migration.md`。包级文档：`extend/pull/README.md`（架构/配置/存储布局/查询/合并/自定义 Unit）。

- **架构**：`pull.Unit` 接口（`Name()/Codes(s)/FetchDay(s,code)/FetchMin(s,code)`）实现市场可插拔；`pull.Service` 编排（并发 `Goroutines`、重试 `Retry`、增量去重 `Updated`、进度条）；`pull.Config`（2026-09 由 `PullConfig` 更名，消除包名 stuttering）全部通过**代码参数**传入（本库是第三方引用库，不写配置文件）。示例：`examples/data/PullV2`（`go run ./examples/data/PullV2`）。
- **存储（sqlite, `lib/xorms`）**：已确认的股票/ETF/指数/板块及港股日线成交量存为**股**；其余扩展市场/周期按 `Market.Info()` 默认倍率存储，实际单位待确认，不能称为股或合约数。两级「地区/资产」目录：日线 `{Dir}/{地区}/{资产}/day/{code}.db`（一代码一库）；1分钟线 `{Dir}/{地区}/{资产}/min/{code}/{code}-{year}.db`（一代码一年一库）。目录映射（`Code.DirName()`=枚举值）：`cn/stock|index|etf|block`、`cn/future`（期货，多交易所共用，地区级）、`hk/stock|index`、`us/stock`。2026-09 布局演进：`day|min 顶层`→`min 文件带代码`→`市场顶层`→`两级地区/资产`（`cn/stock` 等）。`Code.FileName()`=**原始代码**（目录已含市场信息，2026-09 从 key 改为 code），期货含 `/` 代码替换为 `-`（`cff/IF2609`→`cff-IF2609.db`）。`Code.Key()` 仅用于日志/完整键。`KlineDay{Unix pk, OHLC float64(原币种/报价单位), Volume int64(按市场配置换算), Amount, Turnover, FloatStock, TotalStock}`；`KlineMinute`（全称）同字段无股本/换手。仅存日线+1分钟，5/15/30/60分钟、周/月/季/年由 `DayToPeriod`/`MinuteToPeriod`（`merge.go`，直接聚合原始浮点字段）纯内存派生、不落盘。
- **Code.Key() 规则**：cn 四市场→原代码（已带 sh/sz/bj 前缀）；hk→`HK00700`；us→`US.AAPL`；future/hk_index→`cn/future.cff/IF2609` / `hk/index.HSI`（市场值+`.`+代码，key 含 `/` 时文件名替换为 `-`）。`SplitKey` 与 `ParseCode` 共用忽略大小写的市场前缀解析，支持直接拼接与点分隔，完整市场键继续兼容。
- **Market 枚举（2026-09，值=目录路径）**：`pull.Market`（`type Market string`）+ 常量 `MarketAStock/MarketIndex/MarketEtfLof/MarketBlock/MarketFuture/MarketHK/MarketHKIndex/MarketUS`，定义在 `protocol/market.go`，`extend/pull/market.go` 保留类型与常量别名。**枚举值即两级「地区/资产」路径**：`cn/stock|index|etf|block`、`cn/future`、`hk/stock|index`、`us/stock`——`Code.DirName()` 直接返回枚举值（与磁盘布局完全一致）。`Code.Market`、market 子包 Unit 的 `name` 字段均为该类型（`Name()` 返回 `.String()`）。**演进**：初版扁平值（`a_stock`/`hk`/`us`…）→ 改为目录路径形式（用户提议统一）；`hk.00700` 作为港股简称输入兼容；旧 `a_stock.sh600000` 不再对应内置市场，目录形式为 `hk/stock.00700`/`cn/stock.sh600000`（`hk/index.HSI` 同理）。注意与 `protocol.Exchange`（uint8 协议层市场编码）是两套体系。
- **Config.Codes 扁平化 + ParseCode 自动路由（2026-09）**：`Codes` 从 `map[Market][]string` 改为扁平 `[]string`，NewService 一次性 `ParseCode` 解析并按市场分组（`Service.overrides`）。路由规则：sh/sz/bj 前缀可直接拼接或点分隔（前缀忽略大小写）→按谓词分类（ETF>股票>板块>指数）；6 位裸数字→`protocol.AddPrefix` 补前缀后分类（`000001`→股票 `sz000001` 非上证指数；北交所指数 899xxx 补不上前缀特判为 `bj+code`）；5 位数字→港股；纯字母→美股；期货支持 `cffIF2609` / `cff.IF2609` / `cff/IF2609`（czc/dce/shf/gfe/qhz 同理），交易所简称转小写、合约主体保留大小写；`market.code` 完整键也可。已注册的自定义市场支持两种形式，保留其注册名称和代码；注册时禁止大小写重名及占用内置简称。内置市场主体统一经 `protocol.ParseInstrument` 校验，市场分类复用 `Instrument.Market()`；显式完整市场键必须匹配品种，自定义市场代码不套用内置规则。**破坏性变更**：`map[Market][]string` 旧写法编译失败，PullV2 示例已同步。
- **Config.Units 已删除（2026-09，先增后删）**：`Units` 字段经历 `[]Unit`→`[]Market` 后最终**整个删除**——市场完全由 `Codes` 路由决定：`Codes` 空=全量模式（全部注册市场自动发现）；`Codes` 非空=白名单模式（只拉这些代码，拉取范围限定为涉及市场，其余市场不拉）。NewService 中 overrides 分组决定 units 集合。**测试适配**：mockUnit 注册名 "test"，测试用完整键 `"test.sh600000"` 走 ParseCode 路由（`market.code` 形式对任意注册市场可用）。
- **注册的内置 Unit**（`extend/pull/market/`，`init()` 中 `pull.Register`）：`cn/stock`/`cn/index`/`cn/etf`（标准行情7709，走 `Manage`）；`cn/block`（板块指数，`GetBlockDataWithIndex`）；`cn/future`/`hk/stock`/`us/stock`（扩展行情7727，走 `ExPool`）；`hk/index`（港股指数，市场编码27，2026-09 新增）。`Codes` 空时全部拉取；白名单模式只拉涉及市场。
- **港股/美股品种类型混合（2026-09 实测）**：`protocol.ExInstrument` 的 `Category` 与品种类型**无关**（美股全 cat=13 含 1.4 万只：股票/ETF/指数/信托混合，SPY/QQQ/DJIA 均在其中；港股主板 cat=2）。区分只能靠名称关键词（不可靠，DJIA 名"道琼斯BuyWrite"无"指数"字样）。**港股指数独立成市场 27**（恒生系 HZ50xx/HSI/VHSI + 中华系 CES，331 只，全 cat=5）——协议可精确区分，故整市场接入为 `hk/index`。港股 ETF（盈富基金 02800 在市场 49）、美股 ETF/指数（市场 74 混合）暂不拆分。`ParseCode` 白名单 `hkIndexCodes`（HSI/HKL/VHSI/CES100/GEM）优先于美股纯字母判断；港股前缀统一为 `hk`（忽略大小写），5 位纯数字归股票，字母或字母数字归指数，如 `hk.HSI` / `hkHSI` / `hk.CES120`；`hk/index.CES120` 完整键兼容。
- **K 线单位与默认值**：`Market.Info()` 集中记录 pull 日线/1 分钟线的协议、解码分类、请求类型和解码后量单位；股票/ETF 为协议百股（×100），标准指数/板块的 KindIndex 日线及分钟解码均已 ×100，为股，存储不能再乘。未知扩展市场/周期按用户要求使用 `VolumeDefault` 暂沿用 ×100，不代表已确认股数或合约数；不把 K 线配置套用到报价、分笔或本地文件。协议百股与港股每手实际交易股数无关。
- **港股日线请求口径**：腾讯控股真实原始响应及独立源交叉验证确认 category=4 的 Trade 为整数百股，单根与多根同日记录一致；category=9 非首条量近似万股、首条为 0xABC9（官方语义未知），不能按百股换算。适配器用 4，不能只改乘数，也不能据此统一港股指数、美股或所有扩展接口。
- **扩展 K线分页细节（当前入口 `GetKline`）**：返回列表**从旧到新**，pull 寻找增量边界时从末尾（最新）往前遍历；`page` 以 800 为步进（`start` 参数是偏移）。日K category：期货和港股主板用 `TypeKlineDay2`(=4)，港股指数及美股保留原有 `TypeKlineDay`(=9)；扩展分钟保留 `TypeKlineMinute2`(=8)，未确认单位的项使用默认值。期货多交易所 market：47中金/60主力/30上期/28郑商/29大商/66广期/42期货指数。**注意 `start` 为 uint16**：翻页偏移超 65535 会回绕，v2 用 `int` 累加 + `exPageLimit` 护栏，超过上限时报错，不保证能拉取全部历史。
- **增量更新**：`tdx.Updated` 按日去重（非交易日 `Workday.TodayIs()` 跳过）；整体键 `"pull:{配置摘要}"` + **按 Unit 粒度键 `"pull:{unit名}:{配置摘要}"`**。配置摘要包含目录、解析后排序的代码白名单、市场集合、Day/Minute、StartAt、TradeFallback，防止不同任务相互跳过；并发数和重试次数不影响任务身份。同一配置某市场中途失败时，下次只重拉该市场，其余跳过；全部成功才标记整体。旧未限定配置的标记保留但不复用，升级后首次重新做增量检查。内置日线适配器过滤起始/增量边界之前的停止记录，按实际返回的最早记录覆盖，保留服务器已不返回的本地前缀；通用 `SaveDay` 仍按显式 `from` 删尾重插。分钟按年文件。首次全量从 `StartAt` 起（未设置默认最近两年，`NewService` 归一化），`last>0` 时增量拉取直到命中边界。**分钟线历史补全**：`FetchMin` 从 `Start.Year()` 到今年逐年检查，文件不存在的年份全量补拉（std/ex 两路径一致）；某年文件中途失败留半库时删除该文件重跑即可。**空数据保护（双重）**：`SaveDay/SaveMin` 空切片直接跳过（不打开库文件，避免创建空库）；`last>0 && len(out)==0`（退市/停牌/异常返回空）跳写，防"只删不插"侵蚀。
- **引擎缓存**：`Service.engines`（map，路径为 key，`sync.Mutex` 保护）缓存已打开的 sqlite 引擎，默认上限 32；引用计数保护使用中的引擎，释放后按最近使用顺序淘汰，`Close()` 统一释放。
- **空分钟历史缓存与时区**：原生扫描成功且返回其他年份数据时，空历史年份记入 `Dir/minute-scans.db`，7 天后复查；不建空行情库、不作永久完成标记，不缓存请求失败或整次空返回，分笔兜底不复用。更早范围与时钟回退不命中。行情年度定位按源日期时分，不能将未知扩展来源按机器时区或固定东八区生成 Unix；真实时间戳与交易日须有来源依据。存储与接口契约见 `docs/v2-migration.md` 的源时间章节。
- **查询 API**：`Service.QueryDay(code,start,end)`（单文件）与 `Service.QueryMinute(code,start,end)`（自动按年定位文件拼接，升序）；start/end 使用源日期／日期时分字符串，空字符串=不限制该端（`rangeQuery` 构造条件，注意 xorms `Where` 返回 `*xorms.Session` 需链式调用）；库不存在返回空不报错。
- **周期合并精度**：pull 的日线、分钟和日历周期直接聚合原始浮点字段，不经过 `protocol.Price` 的整数厘转换；港美股高精度及负价格不能套用标准协议的三位小数尺度。
- **SQLite 批量写入约束**：`insertBatch` 根据字段数和单语句变量预算分批，同一事务提交；`store.go` 的字段数常量须与实际插入字段同步，避免超过 SQLite 占位符上限。
- **updateUnit 失败感知（2026-09，重要）**：`bar.GoRetry` 重试耗尽只吞错误（`Wait()` 无返回值），旧版 `b.Wait(); return nil` 导致**失败的市场也被标记"当日完成"**，下次被去重跳过 → 永久缺数。修复：以"成功计数"判定（注意 GoRetry 的 defer 每次重试尝试都会执行，重试内多次失败会重复计数，故用成功数 `succCount<len(codes)` 判定而非失败次数），失败时返回错误、Update 不标记该市场。教训：upsert 事务回滚只留空表文件（8KB），错误又未传播，表面"拉取完成"实则无数据——双重掩盖。
- **Gbbq 空实现与 pull 懒加载（2026-09，重要坑）**：`NewManage()` 默认 `Gbbq: &Gbbq{}`（manage.go 注释"默认不初始化股本变迁数据"），空实现非 nil 但 db/c/m 全空 → `GetEquity` 恒 nil，拉股票日线时流通股/总股本/换手率**静默为 0**（判 `m.Gbbq != nil` 分辨不出）。区分手段：`Gbbq.IsEmpty()`（db/c/m 全 nil）。股票日线通过 `Manage.InitGbbq()` 检测并替换空实现，初始化与关闭共用管理器互斥锁；自建独立连接、数据库和定时任务由 `Manage.Close()` 统一释放。代码、指数和分钟查询不触发股本初始化。用户传自定义 IGbbq/WithGbbq 时不干预。
- **删尾重插的语义（重要）**：upsert "先删 `>=last` 后插入" 的目的是**自动修复盘中拉到的半根 K 线**（删了完整重插）。两个配套约定：① 拉取时命中边界那根（t == last）必须**收入结果再停**（std 路径 `GetKlineDayUntil` 天然含边界；ex 路径 2026-08 修复前 `break` 前不 append，每次增量静默丢一根——已修复）；② **空结果保护**：`last>0 && len(out)==0`（退市/长期停牌/服务端异常返回空）时必须跳过写入，否则只剩"删"没有"插"，逐次侵蚀历史（a_stock/future 两路径均已加守卫）。

## 踩坑记录

- **分钟线午休时间错位（11:30→13:00）**：用户曾在分笔合成和单日/历史分钟查询中遇到。标准 K 线解码已有 `MKline.Decode → FixKlineTime` 的 13:00→11:30 补偿，但只处理运行日、末条时间在当天 13:00～15:00 内的数据，并且只检查最后 120 根。离线样本确认：历史日期的异常 13:00 不修正；完整 241 根中午休边界位于第 121 根，会被“最后 120 根”排除，因此不能认为历史或完整单日 K 线已全面修复。当前 `Trades.Klines()` 固定保留 11:30、下午从 13:01 开始（11:30 成交归入 11:30，13:00 成交归入 13:01），未复现合成结果把 11:30 改成 13:00；标准 `GetMinute/GetHistoryMinute` 走独立历史分时解码，序号时间同样为 11:30→13:01，不经过 `FixKlineTime`。排查应分别核对原始分笔/分钟时间、交易日期、时区及输出边界；不能向港美股/期货或所有真实 13:00 记录无条件套用沪深修正。原异常服务器样本尚未复核，不据此推断服务端根因。
- **无日线仍返回分笔**：用户遇到过无日线但分笔接口非空且内容异常的情况；不能以分笔非空判断历史数据可用。历史分笔统一先查同代码、同交易日期的日线；缺少对应日线即跳过，检查失败必须报错，不能当作无数据。只查最近一根不足以判断历史日期，需分页定位请求日；整日分笔分页只检查一次。离线模拟确认旧路径缺少此检查；原异常证券/日期尚无真实样本核验。当天实时分笔不套用该历史检查，日线存在也不能证明分笔准确。
- **`.day` 价格是 uint32×(100)** 而非 float32——旧实现用 bytesToFloat 解析价格/成交量完全错误，日期解析也有误（现已修复）。
- **分钟线价格/成交额是 float32**，必须用 `math.Float32frombits` 转，直接 `Uint32` 得位模式会得到天文数字价格。
- **float32 精度限制**：分钟线价格以 float32(元) 存，两位小数价格经 float32 往返会差 ±1厘（如 11.2 元→11.199999），测试断言需容差。
- **`.lc1` 占位记录（与 `.lc5` 的差异，重要）**：真实 `.lc1` 每个交易日末尾（14:59=899分钟，偶见 14:58=898）有 1 条「量=0额=0」的占位记录，其 OHLC 价格=当日最后成交价（四价相同）；当日首条 09:31 是**真实数据**（量额非零），绝不能重复/跳过。`.lc5` 无此占位。`ReadMinute1` 仅对 `.lc1` 跳过「分钟>=898(14:58)且量额全零」的记录（`ReadMinute5` 全保留）；`WriteMinute1` 仅在当日含 **15:00 收盘记录**时于该记录之前补 14:59 占位，盘中或部分日不补（`WriteMinute5` 不补）。注意：**不能只按「量=0额=0」判断占位**——实时拉取的数据里盘中也有量额全零的真实无成交分钟（实测 sz000001 有 101 条），必须同时满足「分钟>=898」才跳过，否则误删。用户生成 `.lc1` 若显示不了成交量，多半是占位/量零记录问题。
- **踩坑（用被覆盖文件误判格式）**：曾用 `sz000001.lc1` 分析真实格式，但它已被本项目 `WriteMinute1` 生成的旧文件覆盖，导致误判「占位在当日首条前、9:31 有重复」。真实格式必须用未被覆盖的文件（如 `sz000002.lc1`）验证。教训：**分析真实格式前先确认样本文件非本工具生成**。
- **1分钟线文件巨大**（如 sz000001.lc1 超 120 万条 / 35MB），读取时注意内存；通达信 1 分钟数据可能较旧（取决于客户端下载范围）。
- `minline/` 目录只有 `.lc1`(1分钟)；5分钟 `.lc5` 实际存放在 `fzline/` 目录。
- zhb.zip 盘后包包含 tdxstat/tdxstat2/tdxbjmore/tdxhy/tdxzs/tdxbk/gbbq 等文件，数量随来源更新；文本通常为 GBK，gbbq 等二进制文件须使用各自解码器。现有 `UTF8ToGBK` 名称与方向相反，实际调用 GBK 解码器，转换为 UTF-8 并去除 NUL；不要按函数名误判转换方向。
