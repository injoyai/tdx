package model

// MinuteEmptyScan 记录成功扫描为空的历史范围，不表示数据完整。
type MinuteEmptyScan struct {
	From      string
	CheckedAt int64 // 缓存检查时间，Unix 秒；并非行情源时间
}
