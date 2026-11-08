package pull

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
)

// QueryMinuteCoverage 查询分钟扫描覆盖状态；无记录返回 nil。
func (s *Service) QueryMinuteCoverage(code model.Code, year int) (*model.MinuteCoverage, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	return s.store.QueryMinuteCoverage(code, year)
}

// MinuteYearComplete 保留原生分钟扫描语义；分笔兜底还需检查 TradeFallback 与 Through。
func (s *Service) MinuteYearComplete(code model.Code, year int, from string) (bool, error) {
	if err := model.ValidateTimeRange(from, "", "2006-01-02 15:04"); err != nil {
		return false, err
	}
	c, err := s.QueryMinuteCoverage(code, year)
	through := time.Date(year+1, 1, 1, 0, 0, 0, 0, s.Start().Location()).Format("2006-01-02 15:04")
	return c != nil && c.From <= from && (!c.TradeFallback || c.Through >= through), err
}

// SaveMinuteComplete 将原生历史扫描数据及完成标记原子提交，空数据不写标记。
func (s *Service) SaveMinuteComplete(code model.Code, year int, from string, ks []*model.KlineMinute) error {
	return s.saveMinuteCoverage(code, year, ks, model.MinuteCoverage{From: from})
}

// SaveMinuteFallbackComplete 提交原生与合成数据，以及 [from,through) 已检查标记。
// 调用方须先处理所有缺失交易日，并将需保留的本地尾部数据合并到 ks。
func (s *Service) SaveMinuteFallbackComplete(code model.Code, year int, from, through string, ks []*model.KlineMinute) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(ks) == 0 {
		return nil
	}
	if err := model.ValidateTimeRange(from, through, "2006-01-02 15:04"); err != nil {
		return err
	}
	if through <= from {
		return fmt.Errorf("pull: 无效分钟覆盖区间 %s..%s", from, through)
	}
	return s.saveMinuteCoverage(code, year, ks, model.MinuteCoverage{
		From: from, Through: through, TradeFallback: true,
	})
}

func (s *Service) saveMinuteCoverage(code model.Code, year int, ks []*model.KlineMinute, coverage model.MinuteCoverage) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(ks) == 0 {
		return nil
	}
	if err := model.ValidateMinutes(year, coverage.From, ks); err != nil {
		return err
	}
	writeFrom := ks[0].Time
	for _, k := range ks {
		writeFrom = min(writeFrom, k.Time)
	}
	return s.store.SaveMinute(code, year, writeFrom, ks, &coverage)
}
