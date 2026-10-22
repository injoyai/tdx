package protocol

import "time"

var location = time.FixedZone("UTC+08:00", 8*60*60)

// Location 返回行情时间使用的固定东八区，不依赖运行环境的 TZ。
// 扩展行情的钟面时间仍按原值保留，不推断证券当地时区或夜盘自然日。
func Location() *time.Location { return location }
