package extend

type THSFactor struct {
	Date    int64   `json:"date"`     //时间
	QFactor float64 `json:"q_factor"` //前复权因子
	HFactor float64 `json:"h_factor"` //后复权因子
}

type Factor struct {
	Date    int64   `json:"date"`     //时间
	QFactor float64 `json:"q_factor"` //前复权因子
	HFactor float64 `json:"h_factor"` //后复权因子
}
