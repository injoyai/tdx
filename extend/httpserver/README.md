# tdx HTTP Server

将通达信(tdx)行情数据通过 HTTP API 对外开放。本包基于 `tdx.Client` 实现,以 RESTful GET 接口暴露股票、指数、扩展行情等数据。

## 快速开始

```go
package main

import (
	"log"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/httpserver"
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
		httpserver.WithExHqHosts(tdx.ExHosts...), // 可选,启用扩展行情 /ex/* 路由
		httpserver.WithOptions(tdx.WithRedial()),
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
| `WithExHqHosts(hosts...)` | 扩展行情服务器列表,为空则不启用扩展行情 | 无 |
| `WithExPoolSize(n)` | 扩展连接池大小 | `1` |
| `WithOptions(opts...)` | 通达信连接选项,如 `tdx.WithDebug()`、`tdx.WithRedial()` | 无 |

> `Default()` 会自动添加 `tdx.WithRedial()` 断线重连选项。

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
| `GET /count` | `exchange` | 获取指定交易所的证券数量 |
| `GET /code` | `exchange`, `start` | 获取指定交易所的证券代码(分页) |
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

### K线(股票)

| 路径 | 参数 | 说明 |
| --- | --- | --- |
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

### 指数K线

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /index` | `type`, `code`, `start`, `count` | 获取指定类型的指数 K 线(分页) |
| `GET /index/all` | `type`, `code` | 获取指定类型的全部指数 K 线 |
| `GET /index/minute` | `code`, `start`, `count` | 获取指数 1 分钟 K 线(分页) |
| `GET /index/5minute` | `code`, `start`, `count` | 获取指数 5 分钟 K 线(分页) |
| `GET /index/15minute` | `code`, `start`, `count` | 获取指数 15 分钟 K 线(分页) |
| `GET /index/30minute` | `code`, `start`, `count` | 获取指数 30 分钟 K 线(分页) |
| `GET /index/60minute` | `code`, `start`, `count` | 获取指数 60 分钟 K 线(分页) |
| `GET /index/day` | `code`, `start`, `count` | 获取指数日 K 线(分页) |
| `GET /index/day/all` | `code` | 获取全部指数日 K 线 |
| `GET /index/week/all` | `code` | 获取全部指数周 K 线 |
| `GET /index/month/all` | `code` | 获取全部指数月 K 线 |
| `GET /index/quarter/all` | `code` | 获取全部指数季 K 线 |
| `GET /index/year/all` | `code` | 获取全部指数年 K 线 |

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

### 扩展行情

> 需要通过 `WithExHqHosts()` 选项启用,否则返回 404。

| 路径 | 参数 | 说明 |
| --- | --- | --- |
| `GET /ex/markets` | 无 | 获取扩展行情市场列表 |
| `GET /ex/count` | 无 | 获取扩展行情证券数量 |
| `GET /ex/instruments` | `start`, `count` | 获取扩展行情证券列表(分页) |
| `GET /ex/quote` | `market`, `code` | 获取扩展行情实时报价 |
| `GET /ex/quote_list` | `market`, `category`, `start`, `count` | 获取扩展行情报价列表(分页) |
| `GET /ex/bars` | `category`, `market`, `code`, `start`, `count` | 获取扩展行情 K 线(分页) |
| `GET /ex/minute` | `market`, `code` | 获取扩展行情分时数据 |
| `GET /ex/minute/hist` | `market`, `code`, `date` | 获取扩展行情历史分时数据 |
| `GET /ex/trade` | `market`, `code`, `start`, `count` | 获取扩展行情分笔成交(分页) |
| `GET /ex/trade/hist` | `market`, `code`, `date`, `start`, `count` | 获取扩展行情历史分笔成交(分页) |
| `GET /ex/bars/range` | `market`, `code`, `date`, `date2` | 获取扩展行情指定日期区间 K 线 |

## 参数说明

| 参数 | 说明 | 示例 |
| --- | --- | --- |
| `exchange` | 交易所代码,可选 `sh`(上海)、`sz`(深圳)、`bj`(北京) | `sh` |
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
| `4` | 日K(变体,数值需除以 100) |
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
curl "http://localhost:8080/ex/markets"

# 获取扩展行情报价
curl "http://localhost:8080/ex/quote?market=47&code=600519"
```
