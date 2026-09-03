package pull

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/injoyai/bar"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/lib/xorms"
)

// Service 拉取服务：编排市场 Unit、增量去重、并发、重试、落库。
type Service struct {
	cfg       *Config
	units     []Unit
	updated   *tdx.Updated
	updatedDB *xorms.Engine // 内部自建的去重库，Close 时释放
	day       bool
	minute    bool
	retry     int
	goro      int
	start     time.Time

	// Config.Codes 解析结果：市场 → 代码列表（一次性解析，Update 时按市场取用）
	overrides map[Market][]Code

	mu      sync.Mutex               // 保护 engines 缓存
	engines map[string]*xorms.Engine // 已打开的 sqlite 引擎缓存（key=文件路径）
}

// NewService 构建拉取服务。配置通过代码参数传入（本库作为第三方引用，不写配置文件）。
func NewService(cfg *Config) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("pull: 配置不能为 nil")
	}
	if cfg.Dir == "" {
		return nil, fmt.Errorf("pull: 数据根目录 Dir 不能为空")
	}

	// 归一化默认值
	day := cfg.Day
	minute := cfg.Minute
	if !day && !minute {
		day, minute = true, true
	}
	goro := cfg.Goroutines
	if goro <= 0 {
		goro = 8
	}
	retry := cfg.Retry
	if retry <= 0 {
		retry = tdx.DefaultRetry
	}

	// 代码列表：一次性解析并按市场分组。
	// Codes 非空 = 白名单模式：只拉这些代码（拉取范围限定为涉及的市场）；
	// Codes 为空 = 全量模式：全部注册市场各自自动发现代码。
	overrides := map[Market][]Code{}
	if len(cfg.Codes) > 0 {
		for _, s := range cfg.Codes {
			c, err := ParseCode(s)
			if err != nil {
				return nil, err
			}
			overrides[c.Market] = append(overrides[c.Market], c)
		}
	}

	// 需要更新的市场：白名单模式取涉及的市场，全量模式取全部注册
	var units []Unit
	if len(overrides) > 0 {
		for m := range overrides {
			u, ok := Get(m.String())
			if !ok {
				return nil, fmt.Errorf("pull: 未注册的市场 %s", m)
			}
			units = append(units, u)
		}
	} else {
		units = Units()
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("pull: 未注册任何市场 Unit")
	}

	// 增量去重库：未指定则自动创建于 Dir/updated.db
	updated := cfg.Updated
	var updatedDB *xorms.Engine
	if updated == nil {
		db, err := xorms.NewSqlite(filepath.Join(cfg.Dir, "updated.db"))
		if err != nil {
			return nil, err
		}
		updated, err = tdx.NewUpdated(db, 15, 1)
		if err != nil {
			db.Close()
			return nil, err
		}
		updatedDB = db
	}

	// 起始日期：未设置时默认最近两年（首拉全量的最早日期）
	start := cfg.Start()
	if start.IsZero() {
		now := time.Now()
		start = time.Date(now.Year()-2, now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	}

	return &Service{
		cfg:       cfg,
		units:     units,
		updated:   updated,
		updatedDB: updatedDB,
		day:       day,
		minute:    minute,
		retry:     retry,
		goro:      goro,
		start:     start,
		overrides: overrides,
		engines:   map[string]*xorms.Engine{},
	}, nil
}

// Close 关闭内部持有的增量去重库连接（外部传入的 Updated/连接池由调用方自行管理）。
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for path, db := range s.engines {
		_ = db.Close()
		delete(s.engines, path)
	}
	if s.updatedDB == nil {
		return nil
	}
	return s.updatedDB.Close()
}

// Config 返回配置副本（只读；Codes 切片拷贝，避免与 Service 共享底层）。
func (s *Service) Config() Config {
	c := *s.cfg
	if s.cfg.Codes != nil {
		c.Codes = append([]string(nil), s.cfg.Codes...)
	}
	return c
}

// Start 返回起始日期。
func (s *Service) Start() time.Time { return s.start }

// Manage 返回标准行情(7709)连接源；未配置返回 nil。
func (s *Service) Manage() *tdx.Manage { return s.cfg.Manage }

// ExPool 返回扩展行情(7727)连接池；未配置返回 nil。
func (s *Service) ExPool() tdx.IPool { return s.cfg.ExPool }

// Update 执行一次拉取。must=true 时忽略工作日与当日去重，强制拉取。
// 按 Unit 粒度去重：某市场完成后单独标记，中途失败的市场下次重拉，已完成的跳过。
func (s *Service) Update(ctx context.Context, must ...bool) error {
	if len(must) == 0 || !must[0] {
		// 非交易日直接跳过（由调用方传入 Workday 判断）
		if wd, ok := s.workday(); ok {
			if !wd.TodayIs() {
				return nil
			}
		}
		// 当日已拉过则跳过（整体增量去重）
		done, err := s.updated.Updated("pull")
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}

	var firstErr error
	for _, u := range s.units {
		key := "pull:" + u.Name()
		done, err := s.updated.Updated(key)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if done {
			continue
		}
		if err := s.updateUnit(ctx, u); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.updated.Update(key); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	// 全部完成，标记当日已拉取
	return s.updated.Update("pull")
}

// workday 若调用方配置了交易日对象则返回，否则 ok=false。
func (s *Service) workday() (*tdx.Workday, bool) {
	if s.cfg.Workday != nil {
		return s.cfg.Workday, true
	}
	return nil, false
}

// updateUnit 拉取单个市场。返回错误表示存在失败代码（已重试耗尽），
// Update 据此不标记该市场"当日完成"，下次重跑会继续补拉。
func (s *Service) updateUnit(ctx context.Context, u Unit) error {
	codes, err := s.codes(ctx, u)
	if err != nil {
		return err
	}
	if len(codes) == 0 {
		return nil
	}

	b := bar.NewCoroutine(len(codes), s.goro, bar.WithPrefix(fmt.Sprintf("[%s]", u.Name())))
	defer b.Close()

	// 成功计数：成功数=总数表示全部完成；失败时记录首个错误。
	// 注意 defer 在 GoRetry 每次重试尝试后都会执行，重试成功最终 err=nil 不计数，
	// 故以"成功数"而非"失败次数"判定（避免重试内多次失败重复计数）。
	var succCount int64
	var mu sync.Mutex
	var firstFail error

	for _, code := range codes {
		code := code
		b.GoRetry(func() (err error) {
			b.SetPrefix(fmt.Sprintf("[%s] %s", u.Name(), code.Key()))
			b.Flush()
			defer func() {
				if err == nil {
					atomic.AddInt64(&succCount, 1)
					return
				}
				mu.Lock()
				if firstFail == nil {
					firstFail = err
				}
				mu.Unlock()
				b.Logf("[错误] [%s] %s\n", code.Key(), err)
				b.Flush()
			}()
			if s.day {
				if err = u.FetchDay(ctx, s, code); err != nil {
					return err
				}
			}
			if s.minute {
				if err = u.FetchMin(ctx, s, code); err != nil {
					return err
				}
			}
			return nil
		}, s.retry)
	}
	b.Wait()
	if n := atomic.LoadInt64(&succCount); n < int64(len(codes)) {
		return fmt.Errorf("pull: 市场 %s 拉取不完整（成功 %d/%d），首个错误: %v",
			u.Name(), n, len(codes), firstFail)
	}
	return nil
}

// codes 获取市场代码列表；Config.Codes 指定时优先使用（已按市场分组）。
// 注意：overrides 只包含用户显式列出的市场；其他市场仍走自动发现。
func (s *Service) codes(ctx context.Context, u Unit) ([]Code, error) {
	if override, ok := s.overrides[Market(u.Name())]; ok {
		return override, nil
	}
	return u.Codes(ctx, s)
}

// 存储相关快捷方法（供 Unit 实现使用）。

// engine 打开（或复用缓存的）sqlite 引擎并建表；路径为缓存 key。
func (s *Service) engine(filename string, table any) (*xorms.Engine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db, ok := s.engines[filename]; ok {
		return db, nil
	}
	db, err := openDB(filename, table)
	if err != nil {
		return nil, err
	}
	s.engines[filename] = db
	return db, nil
}

// openDay 打开某代码日线库并建表（带引擎缓存）。
func (s *Service) openDay(code Code) (*xorms.Engine, error) {
	return s.engine(code.DayFile(s.cfg.Dir), new(KlineDay))
}

// openMin 打开某代码指定年份分钟线库并建表（带引擎缓存）。
func (s *Service) openMin(code Code, year int) (*xorms.Engine, error) {
	return s.engine(code.MinFile(s.cfg.Dir, year), new(KlineMinute))
}

// DayFile 返回某代码日线文件路径（供 Unit/查询用）。
func (s *Service) DayFile(code Code) string { return code.DayFile(s.cfg.Dir) }

// MinFile 返回某代码指定年份分钟线文件路径。
func (s *Service) MinFile(code Code, year int) string { return code.MinFile(s.cfg.Dir, year) }

// QueryDay 查询某代码日线，按时间范围（升序）；库不存在时返回空。
// start/end 为零值时表示不限制该端。
func (s *Service) QueryDay(code Code, start, end time.Time) ([]*KlineDay, error) {
	if !exists(code.DayFile(s.cfg.Dir)) {
		return nil, nil
	}
	db, err := s.openDay(code)
	if err != nil {
		return nil, err
	}
	return queryDay(db, start, end)
}

// LastDayUnix 查询某代码日线库内最后一条记录的时间戳；空库返回 0。
func (s *Service) LastDayUnix(code Code) (int64, error) {
	db, err := s.openDay(code)
	if err != nil {
		return 0, err
	}
	return LastUnix(db, new(KlineDay))
}

// LastMinUnix 查询某代码指定年份分钟库内最后一条记录的时间戳；空库返回 0。
func (s *Service) LastMinUnix(code Code, year int) (int64, error) {
	db, err := s.openMin(code, year)
	if err != nil {
		return 0, err
	}
	return LastUnix(db, new(KlineMinute))
}

// SaveDay 打开某代码日线库并增量写入（删除 Unix>=from 后插入，事务内幂等）。
// 空数据直接跳过（不打开库文件，避免创建空库）。
func (s *Service) SaveDay(code Code, from int64, ks []*KlineDay) error {
	if len(ks) == 0 {
		return nil
	}
	db, err := s.openDay(code)
	if err != nil {
		return err
	}
	return upsertDay(db, from, ks)
}

// SaveMin 打开某代码指定年份分钟库并增量写入（删除 Unix>=from 后插入，事务内幂等）。
// 空数据直接跳过（不打开库文件，避免创建空库）。
func (s *Service) SaveMin(code Code, year int, from int64, ks []*KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	db, err := s.openMin(code, year)
	if err != nil {
		return err
	}
	return upsertMin(db, from, ks)
}

// MinExists 判断某代码指定年份的分钟线文件是否已存在。
func (s *Service) MinExists(code Code, year int) bool {
	return exists(code.MinFile(s.cfg.Dir, year))
}

// QueryMin 查询某代码分钟线，自动按年定位文件并拼接（升序）；不存在的年份跳过。
// start/end 为零值时表示不限制该端。
func (s *Service) QueryMin(code Code, start, end time.Time) ([]*KlineMinute, error) {
	out := []*KlineMinute{}
	startYear, endYear := minMaxYear(start, end)
	for y := startYear; y <= endYear; y++ {
		file := code.MinFile(s.cfg.Dir, y)
		if !exists(file) {
			continue
		}
		db, err := s.engine(file, new(KlineMinute))
		if err != nil {
			return nil, err
		}
		ls, err := queryMin(db, start, end)
		if err != nil {
			return nil, err
		}
		out = append(out, ls...)
	}
	return out, nil
}

// minMaxYear 返回查询区间覆盖的年份范围；零值端点不限制。
func minMaxYear(start, end time.Time) (int, int) {
	startYear, endYear := 1990, time.Now().Year()
	if !start.IsZero() {
		startYear = start.Year()
	}
	if !end.IsZero() {
		endYear = end.Year()
	}
	return startYear, endYear
}

func exists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}
