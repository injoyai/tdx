package model

// MinuteCoverage 记录已处理的请求范围；不保证服务器提供的数据没有缺笔。
// TradeFallback 区分原生扫描与分笔补充，Through 是已处理范围的右开端点。
type MinuteCoverage struct {
	From          string // 源日期时分 YYYY-MM-DD HH:MM
	Through       string // 同一源时间口径的右开端点；原生扫描为空
	TradeFallback bool
}
