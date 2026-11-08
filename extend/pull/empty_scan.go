package pull

import (
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
)

// MinuteYearRecentlyEmpty 检查最近 7 天的空历史扫描；更早范围和时钟回退不复用。
func (s *Service) MinuteYearRecentlyEmpty(code model.Code, year int, from string, now time.Time) (bool, error) {
	if err := s.checkOpen(); err != nil {
		return false, err
	}
	if err := model.ValidateTimeRange(from, "", "2006-01-02 15:04"); err != nil {
		return false, err
	}
	scan, err := s.store.LoadMinuteEmptyScan(code, year)
	if err != nil || scan == nil {
		return false, err
	}
	age := now.Sub(time.Unix(scan.CheckedAt, 0))
	return scan.From <= from && age >= 0 && age < 7*24*time.Hour, nil
}

// SaveMinuteEmptyScan 仅记录成功的空历史扫描，不能缓存失败请求。
func (s *Service) SaveMinuteEmptyScan(code model.Code, year int, from string, now time.Time) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if err := model.ValidateTimeRange(from, "", "2006-01-02 15:04"); err != nil {
		return err
	}
	return s.store.SaveMinuteEmptyScan(code, year, model.MinuteEmptyScan{From: from, CheckedAt: now.Unix()})
}
