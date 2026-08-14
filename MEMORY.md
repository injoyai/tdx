# MEMORY.md — 项目记忆文档

> 本文件按 AGENTS.md 第 4.5 节要求维护，记录项目演进过程中的关键结构、约定、决策与踩坑，避免协作时反复回溯。

## 项目概况

- 通达信(tdx)协议 Go 实现：标准行情(7709端口) + 盘后数据(report file) + 扩展行情(exhq)。
- 支持沪深北交易所、K线/分时/成交、板块、财务、复权、行业归属、报表配置等。

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
6. **退市股票**：强制退市→新三板(代码400/420，市场44)；通达信已清退原代码K线数据，线上查不到退市股K线。

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
- API：`ReadDay(dir, code)`、`ReadMinute(dir, code, MinuteType1|MinuteType5)`；code 需带交易所前缀(如 `sz000001`)，本地文件名为 `sz000001.day` 格式。

## 踩坑记录

- **`.day` 价格是 uint32×(100)** 而非 float32——旧实现用 bytesToFloat 解析价格/成交量完全错误，日期解析也有误（现已修复）。
- **分钟线价格/成交额是 float32**，必须用 `math.Float32frombits` 转，直接 `Uint32` 得位模式会得到天文数字价格。
- **1分钟线文件巨大**（如 sz000001.lc1 超 120 万条 / 35MB），读取时注意内存；通达信 1 分钟数据可能较旧（取决于客户端下载范围）。
- `minline/` 目录只有 `.lc1`(1分钟)；5分钟 `.lc5` 实际存放在 `fzline/` 目录。
- zhb.zip 盘后包内含 46 个配置（tdxstat/tdxstat2/tdxbjmore/tdxhy/tdxzs/tdxbk/gbbq 等），GBK 文本，解析需 `UTF8ToGBK`。