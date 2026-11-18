package market

const (
	TypeKline5Minute  uint8 = 0  // 5分钟K 线
	TypeKline15Minute uint8 = 1  // 15分钟K 线
	TypeKline30Minute uint8 = 2  // 30分钟K 线
	TypeKline60Minute uint8 = 3  // 60分钟K 线
	TypeKlineHour     uint8 = 3  // 1小时K 线
	TypeKlineDay2     uint8 = 4  // 日K 线, 发现和Day的区别是这个要除以100,其他未知
	TypeKlineWeek     uint8 = 5  // 周K 线
	TypeKlineMonth    uint8 = 6  // 月K 线
	TypeKlineMinute   uint8 = 7  // 1分钟
	TypeKlineMinute2  uint8 = 8  // 1分钟K 线,未知啥区别
	TypeKlineDay      uint8 = 9  // 日K 线
	TypeKlineQuarter  uint8 = 10 // 季K 线
	TypeKlineYear     uint8 = 11 // 年K 线
)

const (
	KindIndex = "index"
	KindStock = "stock"
)

// IsIntraday 判断 K 线周期是否携带时分。
func IsIntraday(category uint8) bool {
	switch category {
	case TypeKlineMinute, TypeKlineMinute2, TypeKline5Minute, TypeKline15Minute, TypeKline30Minute, TypeKline60Minute:
		return true
	}
	return false
}
