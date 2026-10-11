package pull

// 这里的手为协议百股单位；不同市场及周期的 K 线换算由 Market.Info 决定。

// ToShares 将"手"转换为"股"。
func ToShares(lots int64) int64 { return lots * 100 }

// FromShares 将"股"还原为"手"（用于协议请求等需要手单位的场景）。
func FromShares(shares int64) int64 { return shares / 100 }
