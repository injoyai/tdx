package pull

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/injoyai/bar"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

// Service 拉取服务：编排市场 Unit、增量去重、并发、重试、落库。
type Service struct {
	cfg       *Config
	units     []Unit
	updated   *update.Tracker
	updatedDB *xorms.Engine // 内部自建的去重库，Close 时释放
	day       bool
	minute    bool
	retry     int
	goro      int
	start     time.Time

	// Config.Codes 解析结果：市场 → 代码列表（一次性解析，Update 时按市场取用）
	overrides map[protocol.Market][]Code

	mu          sync.Mutex               // 保护 engines 缓存
	engines     map[string]*cachedEngine // 有引用计数的 sqlite 引擎缓存
	engineLimit int
	engineClock uint64
	closed      bool

	updateMu    sync.Mutex
	discoveryMu sync.Mutex
	discovering bool
	instruments []protocol.ExInstrument
}

// New 构建拉取服务。配置通过代码参数传入（本库作为第三方引用，不写配置文件）。
func New(cfg *Config) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("pull: 配置不能为 nil")
	}
	if cfg.Dir == "" {
		return nil, fmt.Errorf("pull: 数据根目录 Dir 不能为空")
	}
	start, err := cfg.parseStart()
	if err != nil {
		return nil, fmt.Errorf("pull: 无效起始日期 %q，须为 YYYYMMDD: %w", cfg.StartAt, err)
	}
	// 固定配置快照；连接源等依赖仍由调用方持有，不复制其内部状态。
	snapshot := *cfg
	snapshot.Codes = append([]string(nil), cfg.Codes...)
	cfg = &snapshot

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
	engineLimit := cfg.MaxCachedEngines
	if engineLimit <= 0 {
		engineLimit = 32
	}

	// 代码列表：一次性解析并按市场分组。
	// Codes 非空 = 白名单模式：只拉这些代码（拉取范围限定为涉及的市场）；
	// Codes 为空 = 全量模式：全部注册市场各自自动发现代码。
	overrides := map[protocol.Market][]Code{}
	if len(cfg.Codes) > 0 {
		for _, s := range cfg.Codes {
			c, err := ParseCode(s)
			if err != nil {
				return nil, err
			}
			overrides[c.Market] = append(overrides[c.Market], c)
		}
	}
	for market, codes := range overrides {
		overrides[market] = uniqueCodes(codes)
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
	if err := checkTimeFormat(cfg.Dir); err != nil {
		return nil, err
	}

	// 增量去重库：未指定则自动创建于 Dir/updated.db
	updated := cfg.Tracker
	var updatedDB *xorms.Engine
	if updated == nil {
		db, err := xorms.NewSQLite(filepath.Join(cfg.Dir, "updated.db"))
		if err != nil {
			return nil, err
		}
		updated, err = update.New(db, 15, 1)
		if err != nil {
			db.Close()
			return nil, err
		}
		updatedDB = db
	}

	// 起始日期：未设置时默认最近两年（首拉全量的最早日期）
	if cfg.StartAt == "" {
		now := time.Now().In(protocol.Location())
		start = time.Date(now.Year()-2, now.Month(), now.Day(), 0, 0, 0, 0, protocol.Location())
	}

	return &Service{
		cfg:         cfg,
		units:       units,
		updated:     updated,
		updatedDB:   updatedDB,
		day:         day,
		minute:      minute,
		retry:       retry,
		goro:        goro,
		start:       start,
		overrides:   overrides,
		engines:     map[string]*cachedEngine{},
		engineLimit: engineLimit,
	}, nil
}

// Close 关闭内部持有的增量去重库连接（外部传入的 Tracker/连接池由调用方自行管理）。
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	for path, entry := range s.engines {
		if entry.refs == 0 {
			_ = entry.db.Close()
			delete(s.engines, path)
		}
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

// TradeFallbackEnabled 返回是否启用历史分笔补充分钟线。
func (s *Service) TradeFallbackEnabled() bool { return s.cfg.TradeFallback }

// Start 返回起始日期。
func (s *Service) Start() time.Time { return s.start }

// Manage 返回标准行情(7709)连接源；未配置返回 nil。
func (s *Service) Manage() *manage.Manage { return s.cfg.Manage }

// ExPool 返回扩展行情(7727)连接池；未配置返回 nil。
func (s *Service) ExPool() tdx.ClientPool { return s.cfg.ExPool }

// GetInstruments Update 期间共用一次发现；Update 之外每次重新查询。
func (s *Service) GetInstruments() ([]protocol.ExInstrument, error) {
	s.discoveryMu.Lock()
	defer s.discoveryMu.Unlock()
	if s.discovering && s.instruments != nil {
		return append([]protocol.ExInstrument(nil), s.instruments...), nil
	}
	p := s.ExPool()
	if p == nil {
		return nil, fmt.Errorf("pull: 需要配置 ExPool（扩展行情7727连接池）")
	}
	out := []protocol.ExInstrument{}
	err := p.Do(func(c tdx.Client) error {
		ex, ok := c.(*tdx.ExConn)
		if !ok || ex == nil {
			return fmt.Errorf("pull: 需要扩展行情连接")
		}
		for start := uint32(0); ; {
			r, err := ex.SendFrame(protocol.MEx.FrameInstrument(start, 800))
			if err != nil {
				return err
			}
			ins := r.([]protocol.ExInstrument)
			out = append(out, ins...)
			if len(ins) < 800 {
				return nil
			}
			start += uint32(len(ins))
		}
	})
	if err != nil {
		return nil, err
	}
	if s.discovering {
		s.instruments = out
	}
	return append([]protocol.ExInstrument(nil), out...), nil
}

// Update 执行一次拉取。must=true 时跳过工作日与整体标记检查，仍保留市场级去重。
// 按 Unit 粒度去重：某市场完成后单独标记，中途失败的市场下次重拉，已完成的跳过。
func (s *Service) Update(must ...bool) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	s.discoveryMu.Lock()
	s.discovering = true
	s.instruments = nil
	s.discoveryMu.Unlock()
	defer func() {
		s.discoveryMu.Lock()
		s.discovering = false
		s.instruments = nil
		s.discoveryMu.Unlock()
	}()

	key := s.updateKey("pull")
	if len(must) == 0 || !must[0] {
		// 非交易日直接跳过（由调用方传入 Workday 判断）
		if wd, ok := s.workday(); ok {
			if !wd.TodayIs() {
				return nil
			}
		}
		// 当日已拉过则跳过（整体增量去重）
		done, err := s.updated.IsUpdated(key)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}

	var firstErr error
	for _, u := range s.units {
		key := s.updateKey("pull:" + u.Name())
		done, err := s.updated.IsUpdated(key)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if done {
			continue
		}
		if err := s.updateUnit(u); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.updated.Mark(key); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}

	// 全部完成，标记当日已拉取
	return s.updated.Mark(key)
}

// updateKey 将去重标记限定到实际配置；代码和市场顺序不影响任务身份。
func (s *Service) updateKey(prefix string) string {
	var codes, units []string
	for market, override := range s.overrides {
		for _, code := range override {
			codes = append(codes, market.String()+"."+code.Code)
		}
	}
	for _, unit := range s.units {
		units = append(units, unit.Name())
	}
	sort.Strings(codes)
	sort.Strings(units)
	// 仅字符串、布尔值与字符串切片，Marshal 不会失败。
	data, _ := json.Marshal(struct {
		Dir, StartAt               string
		Day, Minute, TradeFallback bool
		Codes, Units               []string
	}{filepath.Clean(s.cfg.Dir), s.cfg.StartAt, s.day, s.minute, s.cfg.TradeFallback, codes, units})
	return fmt.Sprintf("%s:%x", prefix, sha256.Sum256(data))
}

// workday 若调用方配置了交易日对象则返回，否则 ok=false。
func (s *Service) workday() (*workday.Workday, bool) {
	if s.cfg.Workday != nil {
		return s.cfg.Workday, true
	}
	return nil, false
}

// updateUnit 拉取单个市场；失败重试耗尽后返回错误，不标记该市场完成。
func (s *Service) updateUnit(u Unit) error {
	codes, err := s.codes(u)
	if err != nil {
		return err
	}
	if len(codes) == 0 {
		return nil
	}
	b := bar.NewCoroutine(len(codes), s.goro, bar.WithPrefix(fmt.Sprintf("[%s]", u.Name())))
	defer b.Close()
	var succCount int64
	var mu sync.Mutex
	var firstFail error
	for _, code := range codes {
		code := code
		b.Go(func() {
			b.SetPrefix(fmt.Sprintf("[%s] %s", u.Name(), code.Key()))
			b.Flush()
			fetchWithRetry := func(fetch func(*Service, Code) error) (err error) {
				for attempt := 0; attempt < s.retry; attempt++ {
					err = fetch(s, code)
					if err == nil {
						return nil
					}
					b.Logf("[错误] [%s] %s\n", code.Key(), err)
				}
				return err
			}
			var err error
			if s.day {
				err = fetchWithRetry(u.FetchDay)
			}
			if err == nil && s.minute {
				err = fetchWithRetry(u.FetchMinute)
			}
			if err == nil {
				atomic.AddInt64(&succCount, 1)
				return
			}
			mu.Lock()
			if firstFail == nil {
				firstFail = err
			}
			mu.Unlock()
		})
	}
	b.Wait()
	if n := atomic.LoadInt64(&succCount); n < int64(len(codes)) {
		return fmt.Errorf("pull: 市场 %s 拉取不完整（成功 %d/%d），首个错误: %w",
			u.Name(), n, len(codes), firstFail)
	}
	return nil
}

// codes 获取市场代码列表；Config.Codes 指定时优先使用（已按市场分组）。
// 注意：overrides 只包含用户显式列出的市场；其他市场仍走自动发现。
func (s *Service) codes(u Unit) ([]Code, error) {
	if override, ok := s.overrides[protocol.Market(u.Name())]; ok {
		return override, nil
	}
	codes, err := u.Codes(s)
	if err != nil {
		return nil, err
	}
	return uniqueCodes(codes), nil
}

// uniqueCodes 保留首次出现的名称与顺序，避免同一数据库被重复调度。
func uniqueCodes(codes []Code) []Code {
	out := make([]Code, 0, len(codes))
	seen := make(map[Code]bool, len(codes))
	for _, code := range codes {
		key := Code{Market: code.Market, Code: code.Code}
		if !seen[key] {
			seen[key] = true
			out = append(out, code)
		}
	}
	return out
}

// 存储相关快捷方法（供 Unit 实现使用）。

// openDay 打开某代码日线库并建表（带引擎缓存）。
func (s *Service) openDay(code Code) (*xorms.Engine, func(), error) {
	return s.engine(code.DayFile(s.cfg.Dir), new(KlineDay))
}

// openMinute 打开某代码指定年份分钟线库并建表（带引擎缓存）。
func (s *Service) openMinute(code Code, year int) (*xorms.Engine, func(), error) {
	return s.engine(code.MinuteFile(s.cfg.Dir, year), new(KlineMinute))
}

// DayFile 返回某代码日线文件路径（供 Unit/查询用）。
func (s *Service) DayFile(code Code) string { return code.DayFile(s.cfg.Dir) }

// MinuteFile 返回某代码指定年份分钟线文件路径。
func (s *Service) MinuteFile(code Code, year int) string { return code.MinuteFile(s.cfg.Dir, year) }

// QueryDay 查询某代码日线，按时间范围（升序）；库不存在时返回空。
// start/end 为空字符串时表示不限制该端。
func (s *Service) QueryDay(code Code, start, end string) ([]*KlineDay, error) {
	if err := validateRange(start, end, time.DateOnly); err != nil {
		return nil, err
	}
	if !exists(code.DayFile(s.cfg.Dir)) {
		return nil, nil
	}
	db, release, err := s.openDay(code)
	if err != nil {
		return nil, err
	}
	defer release()
	rows, err := queryDay(db, start, end)
	for _, k := range rows {
		if k.AmountUnknown {
			k.Amount = 0
		}
	}
	return rows, err
}

// LastDayTime 查询某代码日线库内最后一条记录的源时间；空库返回空字符串。
func (s *Service) LastDayTime(code Code) (string, error) {
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
func (s *Service) LastMinuteTime(code Code, year int) (string, error) {
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
func (s *Service) SaveDay(code Code, from string, ks []*KlineDay) error {
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
func (s *Service) SaveMinute(code Code, year int, from string, ks []*KlineMinute) error {
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

// MinuteExists 判断某代码指定年份的分钟线文件是否已存在。
func (s *Service) MinuteExists(code Code, year int) bool {
	return exists(code.MinuteFile(s.cfg.Dir, year))
}

// QueryMinute 查询某代码分钟线，自动按年定位文件并拼接（升序）；不存在的年份跳过。
// start/end 为空字符串时表示不限制该端。
func (s *Service) QueryMinute(code Code, start, end string) ([]*KlineMinute, error) {
	out := []*KlineMinute{}
	if err := validateRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	startYear, endYear := minMaxYear(start, end)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(code.MinuteFile(s.cfg.Dir, 2000)), "*.db"))
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		var y int
		name := filepath.Base(file)
		if len(name) < 7 {
			continue
		}
		if _, err := fmt.Sscanf(name[len(name)-7:], "%d.db", &y); err != nil || y < startYear || y > endYear || file != code.MinuteFile(s.cfg.Dir, y) {
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
		for _, k := range ls {
			if k.AmountUnknown {
				k.Amount = 0
			}
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
