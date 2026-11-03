package pull

import (
	"net"
	"time"

	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/protocol"
)

// Store 是拉取服务的持久化边界，不要求 SQL、文件或指定 ORM。
// 所有方法须支持并发调用，并严格按 Code.Market/Code.Code 隔离证券。
// Identity 是稳定的存储命名空间，用于任务去重；不同数据集不能共用身份，不能包含密码。
// 查询按源时间升序返回，空端点不限范围；无行情返回空、无最后时间返回空字符串。
// SaveDay/SaveMinute 原子删除指定证券 Time>=from 的尾部后插入 rows；分钟只影响指定年。
// coverage 非空时，分钟数据与覆盖标记须原子提交，并合并相容的既有覆盖范围。
// 空数据不删除行情或写完成标记。注入的存储由调用方关闭。
type Store interface {
	update.Store
	Identity() string
	LastDayTime(Code) (string, error)
	QueryDay(code Code, start, end string) ([]*KlineDay, error)
	SaveDay(code Code, from string, rows []*KlineDay) error
	LastMinuteTime(code Code, year int) (string, error)
	QueryMinute(code Code, start, end string) ([]*KlineMinute, error)
	SaveMinute(code Code, year int, from string, rows []*KlineMinute, coverage *MinuteCoverage) error
	QueryMinuteCoverage(code Code, year int) (*MinuteCoverage, error)
	LoadMinuteEmptyScan(code Code, year int) (*MinuteEmptyScan, error)
	SaveMinuteEmptyScan(code Code, year int, scan MinuteEmptyScan) error
}

var _ Store = (*SQLiteStore)(nil)

func (s *Service) checkOpen() error {
	if s.closed.Load() {
		return net.ErrClosed
	}
	return nil
}

func (s *Service) LastDayTime(code Code) (string, error) {
	if err := s.checkOpen(); err != nil {
		return "", err
	}
	return s.store.LastDayTime(code)
}
func (s *Service) LastMinuteTime(code Code, year int) (string, error) {
	if err := s.checkOpen(); err != nil {
		return "", err
	}
	return s.store.LastMinuteTime(code, year)
}
func (s *Service) QueryDay(code Code, start, end string) ([]*KlineDay, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	if err := validateRange(start, end, time.DateOnly); err != nil {
		return nil, err
	}
	return s.store.QueryDay(code, start, end)
}
func (s *Service) QueryMinute(code Code, start, end string) ([]*KlineMinute, error) {
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	if err := validateRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	return s.store.QueryMinute(code, start, end)
}
func (s *Service) SaveDay(code Code, from string, rows []*KlineDay) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if err := validateDays(from, rows); err != nil {
		return err
	}
	return s.store.SaveDay(code, from, rows)
}
func (s *Service) SaveMinute(code Code, year int, from string, rows []*KlineMinute) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	if err := validateMinutes(year, from, rows); err != nil {
		return err
	}
	return s.store.SaveMinute(code, year, from, rows, nil)
}
