package spider

import "sort"

type Factor struct {
	Date    int64   `json:"date"`     //时间
	QFactor float64 `json:"q_factor"` //前复权因子
	HFactor float64 `json:"h_factor"` //后复权因子
}

type Factors []*Factor

func (fs Factors) Get(date int64) *Factor {
	if len(fs) == 0 {
		return &Factor{Date: date, QFactor: 1, HFactor: 1}
	}
	sort.Slice(fs, func(i, j int) bool {
		return fs[i].Date > fs[j].Date
	})
	for _, v := range fs {
		if v.Date <= date {
			return v
		}
	}
	return fs[len(fs)-1]
}
