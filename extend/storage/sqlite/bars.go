package sqlite

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

var _ storage.BarStore = (*PullStore)(nil)
var _ storage.DefinitionStore = (*PullStore)(nil)

// PullStore 保留一代码一日线库、一代码一年一分钟库的存储布局。
// 注入 Service 后仍由调用方关闭。
type PullStore struct {
	dir         string
	updatedDB   *xorm.Engine
	updateMu    sync.Mutex
	mu          sync.Mutex
	engines     map[string]*cachedEngine
	engineLimit int
	engineClock uint64
	closed      bool
}

// NewPull 打开 SQLite 存储；maxCachedEngines<=0 时默认缓存 32 个引擎。
func NewPull(dir string, maxCachedEngines int) (*PullStore, error) {
	if dir == "" {
		return nil, errors.New("pull: 数据根目录 Dir 不能为空")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := checkTimeFormat(dir); err != nil {
		return nil, err
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	db, err := Open(filepath.Join(dir, "updated.db"))
	if err != nil {
		return nil, err
	}
	if err := ensureTable(db, new(updateRecord)); err != nil {
		db.Close()
		return nil, err
	}
	if maxCachedEngines <= 0 {
		maxCachedEngines = 32
	}
	return &PullStore{dir: dir, updatedDB: db, engines: map[string]*cachedEngine{}, engineLimit: maxCachedEngines}, nil
}

func (s *PullStore) Identity() string { return s.dir }

func (s *PullStore) Close() error {
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
func (s *PullStore) openDay(code model.Code) (*xorm.Engine, func(), error) {
	if err := validateCode(code); err != nil {
		return nil, nil, err
	}
	return s.engine(dayFile(code, s.dir), new(model.KlineDay))
}

// openMinute 打开某代码指定年份分钟线库并建表（带引擎缓存）。
func (s *PullStore) openMinute(code model.Code, year int) (*xorm.Engine, func(), error) {
	if err := validateMinuteCode(code, year); err != nil {
		return nil, nil, err
	}
	return s.engine(minuteFile(code, s.dir, year), new(model.KlineMinute))
}

// DayFile 返回某代码日线文件路径；不安全的市场或代码返回空字符串。
func (s *PullStore) DayFile(code model.Code) string { return dayFile(code, s.dir) }

// MinuteFile 返回某代码指定年份分钟线文件路径；非法代码或年份返回空字符串。
func (s *PullStore) MinuteFile(code model.Code, year int) string {
	return minuteFile(code, s.dir, year)
}

// QueryDay 查询某代码日线，按时间范围（升序）；库不存在时返回空，读取失败返回错误。
// start/end 为空字符串时表示不限制该端。
func (s *PullStore) QueryDay(code model.Code, start, end string) ([]*model.KlineDay, error) {
	if err := validateCode(code); err != nil {
		return nil, err
	}
	if err := model.ValidateTimeRange(start, end, time.DateOnly); err != nil {
		return nil, err
	}
	ok, err := fileExists(dayFile(code, s.dir))
	if err != nil {
		return nil, err
	}
	if !ok {
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

// LastDayTime 查询某代码日线库内最后一条记录的源时间；空库返回空字符串，读取失败返回错误。
func (s *PullStore) LastDayTime(code model.Code) (string, error) {
	if err := validateCode(code); err != nil {
		return "", err
	}
	ok, err := fileExists(s.DayFile(code))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	db, release, err := s.openDay(code)
	if err != nil {
		return "", err
	}
	defer release()
	return LastTime(db, new(model.KlineDay))
}

// LastMinuteTime 查询某代码指定年份分钟库内最后一条记录的源时间；空库返回空字符串，读取失败返回错误。
func (s *PullStore) LastMinuteTime(code model.Code, year int) (string, error) {
	if err := validateMinuteCode(code, year); err != nil {
		return "", err
	}
	ok, err := minuteFileExists(code, s.dir, year)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	db, release, err := s.openMinute(code, year)
	if err != nil {
		return "", err
	}
	defer release()
	return LastTime(db, new(model.KlineMinute))
}

// SaveDay 打开某代码日线库并增量写入（删除 Time>=from 后插入，事务内幂等）。
// 空数据直接跳过（不打开库文件，避免创建空库）。
func (s *PullStore) SaveDay(code model.Code, from string, ks []*model.KlineDay) error {
	if err := validateCode(code); err != nil {
		return err
	}
	if len(ks) == 0 {
		return nil
	}
	if err := model.ValidateDays(from, ks); err != nil {
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
func (s *PullStore) saveMinute(code model.Code, year int, from string, ks []*model.KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	if err := model.ValidateMinutes(year, from, ks); err != nil {
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
func (s *PullStore) SaveMinute(code model.Code, year int, from string, ks []*model.KlineMinute, coverage *model.MinuteCoverage) error {
	if err := validateMinuteCode(code, year); err != nil {
		return err
	}
	if len(ks) == 0 {
		return nil
	}
	if err := model.ValidateMinutes(year, from, ks); err != nil {
		return err
	}
	if coverage != nil {
		if err := model.ValidateMinutes(year, coverage.From, ks); err != nil {
			return err
		}
		return s.saveMinuteCoverage(code, year, from, ks, *coverage)
	}
	return s.saveMinute(code, year, from, ks)
}

// MinuteExists 判断某代码指定年份的分钟线文件是否已存在；文件系统错误按不存在处理。
// 需要区分“不存在”和“读取失败”时使用内部带错误返回的检查。
func (s *PullStore) MinuteExists(code model.Code, year int) bool {
	ok, _ := minuteFileExists(code, s.dir, year)
	return ok
}

// QueryMinute 查询某代码分钟线，自动按年定位文件并拼接（升序）；不存在的年份跳过，读取失败返回错误。
// start/end 为空字符串时表示不限制该端。
func (s *PullStore) QueryMinute(code model.Code, start, end string) ([]*model.KlineMinute, error) {
	if err := validateCode(code); err != nil {
		return nil, err
	}
	out := []*model.KlineMinute{}
	if err := model.ValidateTimeRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	startYear, endYear := minMaxYear(start, end)
	files, err := listFiles(filepath.Dir(minuteFile(code, s.dir, 2000)), ".db")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		var y int
		name := filepath.Base(file)
		if len(name) < 7 {
			continue
		}
		if _, err := fmt.Sscanf(name[len(name)-7:], "%d.db", &y); err != nil || y < startYear || y > endYear || file != minuteFile(code, s.dir, y) {
			continue
		}
		db, release, err := s.engine(file, new(model.KlineMinute))
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

// fileExists 返回文件是否存在；仅“确实不存在”视为缺数据，其他文件系统错误返回给调用方。
func fileExists(filename string) (bool, error) {
	_, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// minuteFileExists 检查分钟线文件是否存在，保留文件系统错误。
func minuteFileExists(code model.Code, dir string, year int) (bool, error) {
	return fileExists(minuteFile(code, dir, year))
}

// listFiles 列出目录内指定后缀的普通文件；目录不存在返回空，其他读取错误返回给调用方。
func listFiles(dir, suffix string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	return files, nil
}
