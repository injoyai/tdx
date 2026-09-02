package pull

// 成交量单位约定：协议层原始 Volume 单位 = 手（1手 = 100股）。
// v2 统一在写入数据库前将"手"转换为"股"，上层（查询/合并）无需再区分市场。

// ToShares 将"手"转换为"股"。
func ToShares(lots int64) int64 { return lots * 100 }

// FromShares 将"股"还原为"手"（用于协议请求等需要手单位的场景）。
func FromShares(shares int64) int64 { return shares / 100 }
