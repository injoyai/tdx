package pull

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

// SQLiteStore 保留一代码一日线库、一代码一年一分钟库的存储布局。
// 通过 Config.Store 注入时由调用方关闭；Service 自动创建时由 Service 关闭。
type SQLiteStore struct {
	*update.SQLStore
	dir         string
	updatedDB   *xorms.Engine
	mu          sync.Mutex
	engines     map[string]*cachedEngine
	engineLimit int
	engineClock uint64
	closed      bool
}

// NewSQLiteStore 打开 SQLite 存储；maxCachedEngines<=0 时默认缓存 32 个引擎。
func NewSQLiteStore(dir string, maxCachedEngines int) (*SQLiteStore, error) {
	if dir == "" {
		return nil, errors.New("pull: 数据根目录 Dir 不能为空")
	}
	if err := checkTimeFormat(dir); err != nil {
		return nil, err
	}
	db, err := xorms.NewSQLite(filepath.Join(dir, "updated.db"))
	if err != nil {
		return nil, err
	}
	marks, err := update.NewSQLStore(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	if maxCachedEngines <= 0 {
		maxCachedEngines = 32
	}
	return &SQLiteStore{SQLStore: marks, dir: dir, updatedDB: db, engines: map[string]*cachedEngine{}, engineLimit: maxCachedEngines}, nil
}

func (s *SQLiteStore) Identity() string { return filepath.Clean(s.dir) }

func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var err error
	for path, entry := range s.engines {
		if entry.refs == 0 {
			err = errors.Join(err, entry.db.Close())
			delete(s.engines, path)
		}
	}
	return errors.Join(err, s.updatedDB.Close())
}

// openDay 打开某代码日线库并建表（带引擎缓存）。
func (s *SQLiteStore) openDay(code Code) (*xorms.Engine, func(), error) {
	return s.engine(code.DayFile(s.dir), new(KlineDay))
}

// openMinute 打开某代码指定年份分钟线库并建表（带引擎缓存）。
func (s *SQLiteStore) openMinute(code Code, year int) (*xorms.Engine, func(), error) {
	return s.engine(code.MinuteFile(s.dir, year), new(KlineMinute))
}

// DayFile 返回某代码日线文件路径（供 Unit/查询用）。
func (s *SQLiteStore) DayFile(code Code) string { return code.DayFile(s.dir) }

// MinuteFile 返回某代码指定年份分钟线文件路径。
func (s *SQLiteStore) MinuteFile(code Code, year int) string { return code.MinuteFile(s.dir, year) }

// QueryDay 查询某代码日线，按时间范围（升序）；库不存在时返回空。
// start/end 为空字符串时表示不限制该端。
func (s *SQLiteStore) QueryDay(code Code, start, end string) ([]*KlineDay, error) {
	if err := validateRange(start, end, time.DateOnly); err != nil {
		return nil, err
	}
	if !exists(code.DayFile(s.dir)) {
		return nil, nil
	}
	db, release, err := s.openDay(code)
	if err != nil {
		return nil, err
	}
	defer release()
	rows, err := queryDay(db, start, end)
	return rows, err
}

// LastDayTime 查询某代码日线库内最后一条记录的源时间；空库返回空字符串。
func (s *SQLiteStore) LastDayTime(code Code) (string, error) {
	if !exists(s.DayFile(code)) {
		return "", nil
	}
	db, release, err := s.openDay(code)
	if err != nil {
		return "", err
	}
	defer release()
	return LastTime(db, new(KlineDay))
}

// LastMinuteTime 查询某代码指定年份分钟库内最后一条记录的源时间；空库返回空字符串。
func (s *SQLiteStore) LastMinuteTime(code Code, year int) (string, error) {
	if !s.MinuteExists(code, year) {
		return "", nil
	}
	db, release, err := s.openMinute(code, year)
	if err != nil {
		return "", err
	}
	defer release()
	return LastTime(db, new(KlineMinute))
}

// SaveDay 打开某代码日线库并增量写入（删除 Time>=from 后插入，事务内幂等）。
// 空数据直接跳过（不打开库文件，避免创建空库）。
func (s *SQLiteStore) SaveDay(code Code, from string, ks []*KlineDay) error {
	if len(ks) == 0 {
		return nil
	}
	if err := validateDays(from, ks); err != nil {
		return err
	}
	db, release, err := s.openDay(code)
	if err != nil {
		return err
	}
	defer release()
	return upsertDay(db, from, ks)
}

// SaveMinute 打开某代码指定年份分钟库并增量写入（删除 Time>=from 后插入，事务内幂等）。
// 空数据直接跳过（不打开库文件，避免创建空库）。
func (s *SQLiteStore) saveMinute(code Code, year int, from string, ks []*KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	if err := validateMinutes(year, from, ks); err != nil {
		return err
	}
	db, release, err := s.openMinute(code, year)
	if err != nil {
		return err
	}
	defer release()
	return upsertMin(db, from, ks)
}

// SaveMinute 原子保存分钟数据；coverage 非空时同时合并覆盖标记。
func (s *SQLiteStore) SaveMinute(code Code, year int, from string, ks []*KlineMinute, coverage *MinuteCoverage) error {
	if len(ks) == 0 {
		return nil
	}
	if err := validateMinutes(year, from, ks); err != nil {
		return err
	}
	if coverage != nil {
		if err := validateMinutes(year, coverage.From, ks); err != nil {
			return err
		}
		return s.saveMinuteCoverage(code, year, from, ks, *coverage)
	}
	return s.saveMinute(code, year, from, ks)
}

// MinuteExists 判断某代码指定年份的分钟线文件是否已存在。
func (s *SQLiteStore) MinuteExists(code Code, year int) bool {
	return exists(code.MinuteFile(s.dir, year))
}

// QueryMinute 查询某代码分钟线，自动按年定位文件并拼接（升序）；不存在的年份跳过。
// start/end 为空字符串时表示不限制该端。
func (s *SQLiteStore) QueryMinute(code Code, start, end string) ([]*KlineMinute, error) {
	out := []*KlineMinute{}
	if err := validateRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	startYear, endYear := minMaxYear(start, end)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(code.MinuteFile(s.dir, 2000)), "*.db"))
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		var y int
		name := filepath.Base(file)
		if len(name) < 7 {
			continue
		}
		if _, err := fmt.Sscanf(name[len(name)-7:], "%d.db", &y); err != nil || y < startYear || y > endYear || file != code.MinuteFile(s.dir, y) {
			continue
		}
		db, release, err := s.engine(file, new(KlineMinute))
		if err != nil {
			return nil, err
		}
		ls, err := queryMin(db, start, end)
		release()
		if err != nil {
			return nil, err
		}
		out = append(out, ls...)
	}
	return out, nil
}

// minMaxYear 返回查询区间覆盖的年份范围；空字符串端点不限制。
func minMaxYear(start, end string) (int, int) {
	startYear, endYear := 0, 9999
	if start != "" {
		fmt.Sscanf(start[:4], "%d", &startYear)
	}
	if end != "" {
		fmt.Sscanf(end[:4], "%d", &endYear)
	}
	return startYear, endYear
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
