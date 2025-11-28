package extend

import "sort"

type Factor struct {
	Date    int64   `json:"date"`     //时间
	QFactor float64 `json:"q_factor"` //前复权因子
	HFactor float64 `json:"h_factor"` //后复权因子
}

type Factors []*Factor

func (this Factors) Get(date int64) *Factor {
	if len(this) == 0 {
		return &Factor{Date: date, QFactor: 1, HFactor: 1}
	}
	sort.Slice(this, func(i, j int) bool {
		return this[i].Date > this[j].Date
	})
	for _, v := range this {
		if v.Date <= date {
			return v
		}
	}
	return this[len(this)-1]
}
