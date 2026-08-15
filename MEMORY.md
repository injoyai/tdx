# MEMORY.md — 项目记忆文档

> 本文件按 AGENTS.md 第 4.5 节要求维护，记录项目演进过程中的关键结构、约定、决策与踩坑，避免协作时反复回溯。

## 项目概况

- 通达信(tdx)协议 Go 实现：标准行情(7709端口) + 盘后数据(report file) + 扩展行情(exhq)。
- 支持沪深北交易所、K线/分时/成交、板块、财务、复权、行业归属、报表配置等。
- **输出数据约定**：所有生成/落盘的数据文件统一写入 `./output/`（已入 `.gitignore`），按场景建子目录；禁止散落写入项目根目录/`example/` 内或其他任意路径（见 AGENTS.md 第 8.2 节）。

## 技术栈

- Go 1.23+，依赖 `github.com/injoyai/conv`(类型转换)、`github.com/injoyai/logs`(日志)。
- 参考开源：gotdx / mootdx / pytdx / tdx2db / nieqx-tdx。

## 架构

- `protocol/`：协议帧编解码与数据结构（`model_*.go`、`unit.go`、`frame.go`、`types.go`）。
- `client.go` / `client_exhq.go`：连接与业务 API。
- `extend/`：离线数据读取(`local.go`)、行情拉取、爬虫、HTTP server、指标计算(`model_kline.go`)。
- `lib/`：bse(北交所官网爬虫,已弃用)、gbbq、xorms、zip 工具。
- `example/`：大量独立示例。

## 关键约定与决策记录

1. **北交所市场编码**：`ExchangeBJ=2`（SZ=0/SH=1/BJ=2，通达信官方文档 + gotdx 交叉验证）。
2. **量化枚举齐全**：`protocol/types.go` 已按通达信量化官方 Dict(https://help.tdx.com.cn/quant/docs/markdown/Dict.html) 补全市场类型(ExchangeNQ=44新三板/SHO=8/SZO=9/HK=31/US=74等)、OrderType、PriceType、Status、DividendType、Period 枚举。Exchange 类型是 uint8，注意 `ExchangeINE` 与 `ExchangeSHF` 同为 30。
3. **标准行情协议不支持 BJ 代码列表请求**：`GetCount/GetCode(market=2)` 超时；但行情/K线正常。
4. **北交所代码获取**：`GetCodeAll(ExchangeBJ)` 走 `GetZHBFiles()` → `tdxbjmore.cfg`（含名称，338条，含已分配未上市代码）。弃用官网爬虫（335条，仅已上市）。
5. **tdxstat.cfg**：全市场逐股统计，35字段，无名称字段。名称需另从 `tdxbjmore.cfg`/板块文件等获取。
6. **退市股票**：强制退市→新三板(代码400/420，市场44)；通达信已清退原代码K线数据，线上查不到退市股K线。实测(2026-08)：标准行情7709 `GetKlineDay("600001"/"sh600001")` 返回0条、`GetQuote(sh600001)` 全零；扩展行情7727 `ExQuote/ExBars` 在 market 0/1/2/44 × (原代码/`T600001`) 全部返回全零——标准协议接口均查不到退市股数据(对照组 `sz000609` 正常, 排除网络/协议问题)。客户端能显示退市股(如 `T600001`)是通过【功能→定制品种→退市品种管理】绑定"自设市场+原代码"后，其历史日线缓存于本地 `<安装目录>/T0002/ds_cache/`，文件命名 `<市场标识>#<代码>.~~~day`(实测 `100#T600001.~~~day`/`100#T600003.~~~day`，`100#` 为缓存命名空间/市场标识，`~~~day` 为日线缓存后缀)，非实时行情服务器提供、也不在 `vipdoc/lday` 下。**缓存格式与标准 `.day` 不同**：头部14字节(`[0:10]`保留 + `[10:14]` uint32 记录数)，之后每条32字节——`[0:4]`uint32日期YYYYMMDD、`[4:20]`4×float32开/高/低/收(**元**)、`[20:24]`float32成交额(元)、`[24:28]`uint32成交量(股)、`[28:32]`保留。读取示例：`example/ReadLocalDelisted`。
7. **板块指数(880xxx/881xxx)编码约定**：`protocol.IsIndex/AddPrefix` 已识别板块指数——880xxx(概念/风格/地区)、881xxx(行业)归属上海交易所(ExchangeSH)，`AddPrefix("880741")`→`sh880741`，`IsIndex("sh880741")`=true。板块指数日K线用 `GetIndexDay`(指数解析, 量×100/涨跌家数)、实时行情用 `GetQuote` 均可直接获取；`GetQuote` 已改为对指数类(含板块指数)跳过 `DefaultCodes` 价格修正(指数原始解码价格即正确, 不再要求 DefaultCodes 初始化)。板块指数不在 `GetCodeAll`/`DefaultCodes` 股票代码列表内。示例：`example/GetBlockData` 演示拉全量板块+板块指数行情。
8. **可转债(标准行情7709)**：`protocol.AddPrefix` 已识别可转债前缀——沪市(110/111/113/118)→`sh`、深市(123/125/126/127/128)→`sz`。日K线 `GetKlineDay`/分钟线直接可用(价格解码与股票同路径, 分→厘)。实时行情 `GetQuote` 依赖 `DefaultCodes` 价格修正：转债 `Decimal=4`(价格×10^(2-4)=÷100)，故已收录在市转债价格正确；已退市/未收录转债不在 `DefaultCodes` 内会报"未查询到代码"。当前在市转债约326只(沪深GetCodeAll实时列表可查)。
9. **期货(扩展行情7727)**：`DialExHqDefault` 连通, 走 `client_exhq.go`。合约代码格式=`品种+YYMM`(如 `IF2609`、`A2609`)，`IF00` 等连续/主力代码无效。期货批量行情用 `ExQuoteList(market, 3, 0, n)`(category=3, market: 47中金/60主力期货/30上期/28郑商/29大商/66广期)，返回收/昨结/持仓/量。期货日K用 `ExBars(4, market, code, 0, n)`(扩展行情日K category=4, 与标准行情 Day=9 不同; 时间/价格/持仓/量/结算价全部正确)。`ExInstruments` 分页 start 为全局品种序号(非市场编号), 全市场约14.4万品种, 含通达信商品指数(T001~T032, market=42)。
10. **`DecodeCode` 通用代码解析**：已泛化支持多市场——A股(6位数字自动补前缀, 行为不变)、港股(5位纯数字如 `00700`/`hk00700`)、美股(纯字母如 `AAPL`/`usBRK.B`, 最长前缀匹配避免 `SHOP` 被误拆为 `sh`+`OP`)、期货(需显式前缀如 `cffIF2609`/`dceA2609`, 裸合约如 `IF2609` 因无法确定交易所而报错提示用前缀)。另支持带点后缀格式 `000001.SZ`/`600000.SH`/`00700.HK`/`AAPL.US`/`IF2609.CFF`(后缀=交易所缩写, 大小写均可; 美股点代码 `BRK.B` 因后缀 B 非交易所而按美股代码解析)。前缀支持小写缩写/大写/中文名(如 `上海600000`)。注意: 标准行情7709的 Frame(model_quote/model_kline 等)只接受 A股 6 位定长代码; 港股/美股/期货实际走扩展行情7727(`ExQuote`/`ExBars` 等), 不经 DecodeCode。

## 本地数据文件解析（extend/local.go，参考 pytdx TdxDailyBarReader/TdxLCMinBarReader 官方协议）

- 目录结构：`vipdoc/<sh|sz|bj>/lday|minline|fzline`
- **`.day` 日线**：32字节/条
  - `00~03` uint32 日期(YYYYMMDD)
  - `04~19` 4×uint32 开/高/低/收(价格×100 整数)
  - `20~23` float32 成交额(元)，`24~27` uint32 成交量(股)，`28~31` 保留
- **`.lc1`(1分钟)/`fzline/*.lc5`(5分钟)**：32字节/条
  - `00~01` uint16 日期(year=num/2048+2004, month=(num%2048)/100, day=num%2048%100)
  - `02~03` uint16 当日分钟数(0点起)
  - `04~19` 4×float32 开/高/低/收(元)，`20~23` float32 额(元)，`24~27` uint32 量(股)，`28~31` 保留
- 单位约定：内部 `Price` 单位=厘(元×1000)，`Volume` 单位=手(股÷100，与线上K线一致)。
- **成交量单位差异（指数 vs 股票）**：`.day/.lc1/.lc5` 的 `24~27` 字段，**指数(如 sh000001/sz399001/bj899050)单位是"手"，原值即手**；**股票单位是"股"**，需 ÷100 转手。`ReadDay/ReadMinute1/ReadMinute5/WriteDay/WriteMinute1/WriteMinute5` 已按 `protocol.IsIndex(c)` 区分处理（指数不÷100；写入时股票×100转股、指数原样写手）。判断时用 decodeCode 已带前缀的 c 直接 `IsIndex(c)`，勿再拼前缀。
- API：`ReadDay(dir, code)`、`ReadMinute1(dir, code)`、`ReadMinute5(dir, code)`；code 需带交易所前缀(如 `sz000001`)，本地文件名为 `sz000001.day` 格式。
- **写入**：`WriteDay(code, ks) ([]byte, error)`、`WriteMinute1(code, ks)`、`WriteMinute5(code, ks)`（均 `([]byte, error)`），与读取格式对称，**只返回通达信格式字节流、不落盘**，由调用方自由决定如何写入/使用（如 `example/FetchLC1ForTest` 内自行 `os.WriteFile` 到 `./output/lc1/vipdoc/...`）。code 需带交易所前缀用于判断指数/股票。均在 `extend/local.go`。

## 踩坑记录

- **`.day` 价格是 uint32×(100)** 而非 float32——旧实现用 bytesToFloat 解析价格/成交量完全错误，日期解析也有误（现已修复）。
- **分钟线价格/成交额是 float32**，必须用 `math.Float32frombits` 转，直接 `Uint32` 得位模式会得到天文数字价格。
- **float32 精度限制**：分钟线价格以 float32(元) 存，两位小数价格经 float32 往返会差 ±1厘（如 11.2 元→11.199999），测试断言需容差。
- **`.lc1` 占位记录（与 `.lc5` 的差异，重要）**：真实 `.lc1` 每个交易日末尾（14:59=899分钟，偶见 14:58=898）有 1 条「量=0额=0」的占位记录，其 OHLC 价格=当日最后成交价（四价相同）；当日首条 09:31 是**真实数据**（量额非零），绝不能重复/跳过。`.lc5` 无此占位。`ReadMinute1` 仅对 `.lc1` 跳过「分钟>=898(14:58)且量额全零」的记录（`ReadMinute5` 全保留）；`WriteMinute1` 生成 `.lc1` 时在**当日最后一条真实数据(15:00)之前**补 14:59 占位（`WriteMinute5` 不补）。注意：**不能只按「量=0额=0」判断占位**——实时拉取的数据里盘中也有量额全零的真实无成交分钟（实测 sz000001 有 101 条），必须同时满足「分钟>=898」才跳过，否则误删。用户生成 `.lc1` 若显示不了成交量，多半是占位/量零记录问题。
- **踩坑（用被覆盖文件误判格式）**：曾用 `sz000001.lc1` 分析真实格式，但它已被本项目 `WriteMinute1` 生成的旧文件覆盖，导致误判「占位在当日首条前、9:31 有重复」。真实格式必须用未被覆盖的文件（如 `sz000002.lc1`）验证。教训：**分析真实格式前先确认样本文件非本工具生成**。
- **1分钟线文件巨大**（如 sz000001.lc1 超 120 万条 / 35MB），读取时注意内存；通达信 1 分钟数据可能较旧（取决于客户端下载范围）。
- `minline/` 目录只有 `.lc1`(1分钟)；5分钟 `.lc5` 实际存放在 `fzline/` 目录。
- **分钟线拉取/生成工具**：`example/FetchLC1ForTest/main.go` 实时拉取股票/指数 1分钟与5分钟K线，用 `WriteMinute` 生成 `.lc1/.lc5`，输出统一放 `./output/lc1/vipdoc/<sh|sz>/<minline|fzline>/`（供客户端导入测试，需手动复制到通达信 `vipdoc` 目录）。指数分钟线用 `GetIndexAll(TypeKlineMinute/5Minute)`，股票用 `GetKlineMinuteAll`/`GetKline5MinuteAll`。
- zhb.zip 盘后包内含 46 个配置（tdxstat/tdxstat2/tdxbjmore/tdxhy/tdxzs/tdxbk/gbbq 等），GBK 文本，解析需 `UTF8ToGBK`。