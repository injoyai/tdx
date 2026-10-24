# tdx HTTP Server

将通达信(tdx)行情数据通过 HTTP API 对外开放。本包基于 `tdx.Client` 实现,以 RESTful GET 接口暴露股票、指数、扩展行情等数据。

## 独立代码与股本服务

`ListenCodesHTTP(port, ...codes.Option)` 启动独立代码表服务，提供 `/codes`、`/stocks`、`/etfs`、`/indexes`。`ListenCodesAndGbbqHTTP(port, []codes.Option, []gbbq.Option)` 额外提供 `/gbbqs`。这两个入口返回原始 JSON 列表或映射，不使用下文行情服务的 `code/msg/data` 包装，也不向 `New` 创建的行情服务自动注册路由。

客户端使用 `extend/metadatahttp` 的 `DialCodesHTTP(address, spec...)` 或 `DialGbbqHTTP(address, spec...)`，使用后调用 `Close()` 停止刷新和取消在途请求。服务端在初始化、监听失败或退出时关闭自建缓存，路由按路径匹配，允许携带查询参数。实现见 [服务端入口](metadata.go) 和 [元数据客户端](../metadatahttp/cache.go)。

## 快速开始

```go
package main

import (
	"log"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/httpserver"
)

func main() {
	// 方式一: 默认配置(开启断线重连)
	s, err := httpserver.Default()
	if err != nil {
		log.Fatal(err)
	}

	// 方式二: 自定义配置
	s, err = httpserver.New(
		httpserver.WithAddr(":8080"),
		httpserver.WithPoolSize(2),
		httpserver.WithExHosts(tdx.ExHosts...), // 可选,启用统一接口的扩展市场查询
		httpserver.WithClientOptions(tdx.WithRedial()),
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("服务启动,监听 :8080")
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## 配置选项

使用函数式选项(Functional Options)配置服务:

| 选项函数 | 说明 | 默认值 |
| --- | --- | --- |
| `WithAddr(addr)` | HTTP 监听地址 | `":8080"` |
| `WithHosts(hosts...)` | 标准行情服务器列表 | `tdx.Hosts` |
| `WithPoolSize(n)` | 标准连接池大小 | `1` |
| `WithExHosts(hosts...)` | 扩展行情服务器列表,为空则不启用扩展行情 | 无 |
| `WithExPoolSize(n)` | 扩展连接池大小 | `1` |
| `WithClientOptions(opts...)` | 通达信连接选项,如 `tdx.WithDebug()`、`tdx.WithRedial()` | 无 |

> `Default()` 会自动添加 `tdx.WithRedial()` 断线重连选项。

使用完成后调用 `s.Close()`，同时关闭 HTTP 服务及其标准、扩展行情连接池。初始化失败会释放已创建的连接池。启动时只创建配置的连接池，不再初始化全局 `全局代码缓存`、元数据数据库或额外定时任务；报价所需的精度元数据由标准连接按需获取。

客户端断开或取消请求后，会退出连接池等待，不会继续占用排队名额。已经开始的上游查询仍受通达信客户端请求超时约束。

## 响应格式

所有接口统一返回如下 JSON 结构:

**成功:**

```json
{
  "code": 0,
  "msg": "ok",
  "data": { ... }
}
```

**错误:**

```json
{
  "code": 1,
  "msg": "错误信息",
  "data": null
}
```

## API 路由

### 健康检查

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /` | 无 | 健康检查,返回服务状态 |

### 代码/数量

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /markets` | 无 | 合并标准与已启用扩展池的市场发现结果 |
| `GET /count` | `exchange` | 获取指定交易所的证券数量 |
| `GET /code` | `exchange`, `start`, `count` | 获取指定交易所的证券代码(分页) |
| `GET /code/all` | `exchange` | 获取指定交易所的全部证券代码 |
| `GET /code/stocks` | 无 | 获取全部股票代码 |
| `GET /code/etfs` | 无 | 获取全部 ETF 代码 |
| `GET /code/indexes` | 无 | 获取全部指数代码 |

### 行情/财务

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /quote` | `codes` | 获取实时行情报价(支持多个代码) |
| `GET /call_auction` | `code` | 获取集合竞价数据 |
| `GET /gbbq` | `code` | 获取除权除息(股本变更)数据 |
| `GET /finance` | `exchange`, `code` | 获取财务信息 |
| `GET /company/category` | `exchange`, `code` | 获取公司信息(F10)文件目录 |
| `GET /company/content` | `exchange`, `code`, `filename`, `start`, `length` | 获取公司信息(F10)文件内容 |

### 分时/成交

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /minute` | `code` | 获取当日分时数据 |
| `GET /minute/history` | `date`, `code` | 获取历史分时数据 |
| `GET /trade` | `code`, `start`, `count` | 获取当日分笔成交明细(分页) |
| `GET /trade/all` | `code` | 获取当日全部分笔成交明细 |
| `GET /trade/history` | `date`, `code`, `start`, `count` | 获取历史分笔成交明细(分页) |
| `GET /trade/history/day` | `date`, `code` | 获取指定日期全部分笔成交明细 |

### K线（股票、指数与扩展品种）

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /kline/range` | `code`, `date`, `date2` | 获取包含两端日期的固定一分钟区间 |
| `GET /kline` | `type`, `code`, `start`, `count` | 获取指定类型的 K 线(分页) |
| `GET /kline/all` | `type`, `code` | 获取指定类型的全部 K 线 |
| `GET /kline/minute` | `code`, `start`, `count` | 获取 1 分钟 K 线(分页) |
| `GET /kline/minute/all` | `code` | 获取全部 1 分钟 K 线 |
| `GET /kline/5minute` | `code`, `start`, `count` | 获取 5 分钟 K 线(分页) |
| `GET /kline/5minute/all` | `code` | 获取全部 5 分钟 K 线 |
| `GET /kline/15minute` | `code`, `start`, `count` | 获取 15 分钟 K 线(分页) |
| `GET /kline/15minute/all` | `code` | 获取全部 15 分钟 K 线 |
| `GET /kline/30minute` | `code`, `start`, `count` | 获取 30 分钟 K 线(分页) |
| `GET /kline/30minute/all` | `code` | 获取全部 30 分钟 K 线 |
| `GET /kline/60minute` | `code`, `start`, `count` | 获取 60 分钟 K 线(分页) |
| `GET /kline/60minute/all` | `code` | 获取全部 60 分钟 K 线 |
| `GET /kline/day` | `code`, `start`, `count` | 获取日 K 线(分页) |
| `GET /kline/day/all` | `code` | 获取全部日 K 线 |
| `GET /kline/week` | `code`, `start`, `count` | 获取周 K 线(分页) |
| `GET /kline/week/all` | `code` | 获取全部周 K 线 |
| `GET /kline/month` | `code`, `start`, `count` | 获取月 K 线(分页) |
| `GET /kline/month/all` | `code` | 获取全部月 K 线 |
| `GET /kline/quarter` | `code`, `start`, `count` | 获取季 K 线(分页) |
| `GET /kline/quarter/all` | `code` | 获取全部季 K 线 |
| `GET /kline/year` | `code`, `start`, `count` | 获取年 K 线(分页) |
| `GET /kline/year/all` | `code` | 获取全部年 K 线 |

指数与股票共用 `/kline/*`，按证券代码自动选择解码。

### 板块/报表

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /block/data` | `file` | 获取板块数据(解析后) |
| `GET /block/data/index` | `file` | 获取带索引的板块数据 |
| `GET /block/file` | `file` | 获取板块原始文件内容 |
| `GET /report/file` | `file` | 获取报表文件内容 |
| `GET /zhb/files` | 无 | 获取 ZHB 文件列表 |
| `GET /tdx/zs` | 无 | 获取通达信指数信息 |
| `GET /tdx/bk` | 无 | 获取通达信板块信息 |
| `GET /tdx/stat` | 无 | 获取通达信统计信息 |
| `GET /tdx/stat2` | 无 | 获取通达信统计信息(二) |
| `GET /tdx/xgsg` | 无 | 获取新股申购信息 |
| `GET /tdx/hy` | 无 | 获取通达信行业信息 |
| `GET /spblock` | 无 | 获取特殊板块信息 |

### 扩展协议专用接口

> 需要通过 `WithExHosts()` 选项启用,否则返回 404。

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /ex/count` | 无 | 获取扩展协议全市场证券总数 |
| `GET /ex/instruments` | `start`, `count` | 获取扩展协议全市场证券列表（全局分页） |
| `GET /ex/quote_list` | `market`, `category`, `start`, `count` | 获取扩展行情报价列表(分页) |

## 参数说明

| 参数 | 说明 | 示例 |
| --- | --- | --- |
| `exchange` | 交易所代码,基础行情支持 `sh`、`sz`、`bj`、`hk`、`hi`、`us`、期货交易所；财务/F10 仍限沪深北 | `sh` |
| `code` | 证券代码,可带交易所前缀 | `600519` 或 `sh600519` |
| `codes` | 多个证券代码,逗号分隔 | `sz000001,sh600008` |
| `type` | K 线类型(数字),见下表 | `9` |
| `start` | 起始位置(数字) | `0` |
| `count` | 获取数量(数字) | `100` |
| `date` | 日期,格式 `YYYYMMDD`(如 `20240101`) | `20240101` |
| `market` | 扩展行情市场代码(数字) | `47` |
| `category` | 扩展行情类别(数字) | `1` |
| `file` | 板块/报表文件名 | `block_gn.dat` |
| `filename` | F10 公司信息文件名 | `300052.txt` |
| `length` | 长度(数字) | `5000` |
| `date2` | 结束日期,格式 `YYYYMMDD` | `20240601` |

**K 线类型(`type`)对照表:**

| 值 | 说明 |
| --- | --- |
| `0` | 5 分钟 |
| `1` | 15 分钟 |
| `2` | 30 分钟 |
| `3` | 60 分钟 |
| `4` | 日K别名；统一结果无需再次缩放价格 |
| `5` | 周 |
| `6` | 月 |
| `7` | 1 分钟 |
| `8` | 1 分钟(变体) |
| `9` | 日 |
| `10` | 季 |
| `11` | 年 |

## 使用示例

**获取报价:**

```bash
curl "http://localhost:8080/quote?codes=sz000001,sh600519"
```

**获取日 K 线:**

```bash
# 使用通用接口,指定 type=9(日)
curl "http://localhost:8080/kline?type=9&code=600519&start=0&count=100"

# 使用专用接口
curl "http://localhost:8080/kline/day?code=600519&start=0&count=100"
```

**获取扩展行情:**

```bash
# 获取扩展行情市场列表
curl "http://localhost:8080/markets"

# 获取扩展行情报价
curl "http://localhost:8080/quote?codes=hk00700,cffIF2609"
```

基础行情 HTTP 的 `data` 直接使用 Go Client 的统一模型，不转换回旧协议响应：

| 查询 | data 类型与单位 |
|---|---|
| `/count` | `int`，指定交易所的证券数量 |
| `/code`、`/code/all` | `tdx.CodeResp`；证券代码在 `List[].Instrument.Code` |
| `/markets` | `[]tdx.Exchange`；发现市场不表示该市场已接入统一查询 |
| `/quote` | `[]tdx.Quote`；价格为原币种，成交量见 `VolumeUnit` |
| `/kline/*` | `tdx.KlineResp`；价格为原币种，`Amount=null` 表示金额语义未确认 |
| `/minute/*` | `tdx.MinuteResp` |
| `/trade/*` | `tdx.TradeResp`；扩展价格尺度未确认时 `Price=null` |

单代码查询按证券前缀选择标准或扩展连接池；混合报价保持输入顺序与重复项，任一查询失败返回错误，不返回部分结果。取消请求会退出连接池等待。扩展池未配置时拒绝扩展品种查询。

`/code` 的 `count` 必填且为 1..1000，`start` 为 uint32，表示指定交易所内的偏移。`/count`、`/code` 与原始 `/ex/count`、`/ex/instruments` 的全市场语义不同，后两者是独立的协议发现能力。

`Standard`、`Extended`、`RangeExtended` 保留原始协议数据及其单位；调用方应使用统一字段，只有需要协议细节时才读取原始字段。`code/msg/data` 包装保持一致；原始记录不是成功返回的前提。

已移除 `/index/*`、`/ex/markets`、`/ex/quote`、`/ex/bars`、`/ex/minute*`、`/ex/trade*`、`/ex/bars/range`，不提供重定向。指数改用 `/kline/*`，扩展品种改用带前缀代码的统一接口。
