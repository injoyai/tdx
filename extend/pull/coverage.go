package pull

import (
	"fmt"
	"time"
)

// MinuteCoverage 记录已处理的请求范围；不保证服务器提供的数据没有缺笔。
// TradeFallback 区分原生扫描与分笔补充，Through 是已处理范围的右开端点。
type MinuteCoverage struct {
	ID            int    `xorm:"pk"`
	From          string // 源日期时分 YYYY-MM-DD HH:MM
	Through       string // 同一源时间口径的右开端点；原生扫描为空
	TradeFallback bool
}

// TableName 指定分钟覆盖记录表。
func (MinuteCoverage) TableName() string { return "minuteCoverage" }

// QueryMinuteCoverage 查询分钟扫描覆盖状态；无记录返回 nil。
func (s *Service) QueryMinuteCoverage(code Code, year int) (*MinuteCoverage, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	return s.store.QueryMinuteCoverage(code, year)
}

// MinuteYearComplete 保留原生分钟扫描语义；分笔兜底还需检查 TradeFallback 与 Through。
func (s *Service) MinuteYearComplete(code Code, year int, from string) (bool, error) {
	if err := validateRange(from, "", "2006-01-02 15:04"); err != nil {
		return false, err
	}
	c, err := s.QueryMinuteCoverage(code, year)
	through := time.Date(year+1, 1, 1, 0, 0, 0, 0, s.Start().Location()).Format("2006-01-02 15:04")
	return c != nil && c.From <= from && (!c.TradeFallback || c.Through >= through), err
}

// SaveMinuteComplete 将原生历史扫描数据及完成标记原子提交，空数据不写标记。
func (s *Service) SaveMinuteComplete(code Code, year int, from string, ks []*KlineMinute) error {
	return s.saveMinuteCoverage(code, year, ks, MinuteCoverage{ID: 1, From: from})
}

// SaveMinuteFallbackComplete 提交原生与合成数据，以及 [from,through) 已检查标记。
// 调用方须先处理所有缺失交易日，并将需保留的本地尾部数据合并到 ks。
func (s *Service) SaveMinuteFallbackComplete(code Code, year int, from, through string, ks []*KlineMinute) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(ks) == 0 {
		return nil
	}
	if err := validateRange(from, through, "2006-01-02 15:04"); err != nil {
		return err
	}
	if through <= from {
		return fmt.Errorf("pull: 无效分钟覆盖区间 %s..%s", from, through)
	}
	return s.saveMinuteCoverage(code, year, ks, MinuteCoverage{
		ID: 1, From: from, Through: through, TradeFallback: true,
	})
}

func (s *Service) saveMinuteCoverage(code Code, year int, ks []*KlineMinute, coverage MinuteCoverage) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(ks) == 0 {
		return nil
	}
	if err := validateMinutes(year, coverage.From, ks); err != nil {
		return err
	}
	writeFrom := ks[0].Time
	for _, k := range ks {
		writeFrom = min(writeFrom, k.Time)
	}
	return s.store.SaveMinute(code, year, writeFrom, ks, &coverage)
}
