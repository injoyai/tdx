package pull

import (
	"fmt"
	"net"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func (s *Service) checkOpen() error {
	if s.closed.Load() {
		return net.ErrClosed
	}
	if s.store == nil {
		return fmt.Errorf("pull: 行情同步需要 Store")
	}
	return nil
}

func (s *Service) LastDayTime(code model.Code) (string, error) {
	if err := s.checkOpen(); err != nil {
		return "", err
	}
	return s.store.LastDayTime(code)
}
func (s *Service) LastMinuteTime(code model.Code, year int) (string, error) {
	if err := s.checkOpen(); err != nil {
		return "", err
	}
	return s.store.LastMinuteTime(code, year)
}
func (s *Service) QueryDay(code model.Code, start, end string) ([]*model.KlineDay, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	if err := model.ValidateTimeRange(start, end, time.DateOnly); err != nil {
		return nil, err
	}
	return s.store.QueryDay(code, start, end)
}
func (s *Service) QueryMinute(code model.Code, start, end string) ([]*model.KlineMinute, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	if err := model.ValidateTimeRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	return s.store.QueryMinute(code, start, end)
}
func (s *Service) SaveDay(code model.Code, from string, rows []*model.KlineDay) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if err := model.ValidateDays(from, rows); err != nil {
		return err
	}
	return s.store.SaveDay(code, from, rows)
}
func (s *Service) SaveMinute(code model.Code, year int, from string, rows []*model.KlineMinute) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if err := model.ValidateMinutes(year, from, rows); err != nil {
		return err
	}
	return s.store.SaveMinute(code, year, from, rows, nil)
}
