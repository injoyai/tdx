// Package spider 提供新浪、同花顺数据源及其比例复权因子，与协议仿射因子分开。
package spider

import (
	"net/http"
	"time"
)

var dataHTTPClient = &http.Client{Timeout: 30 * time.Second}
