package pull

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/protocol"
)

// Service 拉取服务：编排市场 Unit、增量去重、并发、重试、落库。
type Service struct {
	cfg            *Config
	units          []Unit
	store          storage.BarStore
	closed         atomic.Bool
	day            bool
	minute         bool
	retry          int
	goro           int
	start          time.Time
	contextMu      sync.RWMutex
	ctx            context.Context
	universeMu     sync.Mutex
	universe       []model.Code
	universeReady  bool
	freezeUniverse bool

	// Config.Codes 解析结果：市场 → 代码列表（一次性解析，Update 时按市场取用）
	overrides map[protocol.Market][]model.Code

	updateGate  chan struct{}
	discoveryMu sync.Mutex
	discovery   *instrumentDiscovery
}

// New 构建拉取服务。配置通过代码参数传入（本库作为第三方引用，不写配置文件）。
func New(cfg *Config) (*Service, error) {
	if cfg == nil {
		return nil, fmt.Errorf("pull: 配置不能为 nil")
	}
	for name, dependency := range map[string]any{
		"Store": cfg.Store, "Pool": cfg.Pool, "ExPool": cfg.ExPool,
	} {
		if dependency != nil && isNil(dependency) {
			return nil, fmt.Errorf("pull: %s 不能包含空对象", name)
		}
	}
	if isNil(cfg.Store) && isNil(cfg.Pool) {
		return nil, fmt.Errorf("pull: 行情同步需要 Store，元数据获取需要 Pool")
	}
	start, err := cfg.parseStart()
	if err != nil {
		return nil, fmt.Errorf("pull: 无效起始日期 %q，须为 YYYYMMDD: %w", cfg.StartAt, err)
	}
	// 固定配置快照；连接源等依赖仍由调用方持有，不复制其内部状态。
	snapshot := *cfg
	snapshot.Codes = append([]string(nil), cfg.Codes...)
	snapshot.Schedules = make(map[protocol.Market]Schedule, len(cfg.Schedules))
	for market, schedule := range cfg.Schedules {
		if _, err := schedule.boundary(time.Now()); err != nil {
			return nil, fmt.Errorf("pull: 市场 %s 调度: %w", market, err)
		}
		snapshot.Schedules[market] = schedule
	}
	if cfg.RetryDelay < 0 {
		return nil, fmt.Errorf("pull: RetryDelay 不能为负")
	}
	cfg = &snapshot
	if isNil(cfg.Store) {
		retry := cfg.Retry
		if retry <= 0 {
			retry = tdx.DefaultRetry
		}
		return &Service{cfg: cfg, start: start, retry: retry, updateGate: make(chan struct{}, 1)}, nil
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
	overrides := map[protocol.Market][]model.Code{}
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
	store := cfg.Store

	if store.Identity() == "" {
		return nil, fmt.Errorf("pull: Store.Identity 不能为空")
	}

	// 起始日期：未设置时默认最近两年（首拉全量的最早日期）
	if cfg.StartAt == "" {
		now := time.Now().In(protocol.Location())
		start = time.Date(now.Year()-2, now.Month(), now.Day(), 0, 0, 0, 0, protocol.Location())
	}

	service := &Service{
		updateGate: make(chan struct{}, 1),
		cfg:        cfg,
		units:      units,
		store:      store,
		day:        day,
		minute:     minute,
		retry:      retry,
		goro:       goro,
		start:      start,
		overrides:  overrides,
	}
	for _, u := range units {
		schedule, err := service.schedule(u)
		if err != nil {
			return nil, err
		}
		if _, err := schedule.boundary(time.Now()); err != nil {
			return nil, fmt.Errorf("pull: 市场 %s 调度: %w", u.Name(), err)
		}
	}
	return service, nil
}

// Close 停止后续业务存储操作；注入的组件均由调用方关闭。
func (s *Service) Close() error { s.closed.Store(true); return nil }

// Config 返回配置副本（只读；Codes 切片拷贝，避免与 Service 共享底层）。
func (s *Service) Config() Config {
	c := *s.cfg
	c.Schedules = make(map[protocol.Market]Schedule, len(s.cfg.Schedules))
	for market, schedule := range s.cfg.Schedules {
		c.Schedules[market] = schedule
	}
	if s.cfg.Codes != nil {
		c.Codes = append([]string(nil), s.cfg.Codes...)
	}
	return c
}

// TradeFallbackEnabled 返回是否启用历史分笔补充分钟线。
func (s *Service) TradeFallbackEnabled() bool { return s.cfg.TradeFallback }

// Start 返回起始日期。
func (s *Service) Start() time.Time { return s.start }

// Pool 返回标准行情(7709)连接池；未配置返回 nil。
func (s *Service) Pool() tdx.ClientPool { return s.cfg.Pool }

// Context returns the active synchronization context. Standalone helpers use Background.
func (s *Service) Context() context.Context {
	s.contextMu.RLock()
	defer s.contextMu.RUnlock()
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// DiscoverCodes reads the universe supplied by ListCodes, fixed for the current Update.
func (s *Service) DiscoverCodes() ([]model.Code, error) {
	s.universeMu.Lock()
	defer s.universeMu.Unlock()
	if s.freezeUniverse && s.universeReady {
		return append([]model.Code(nil), s.universe...), nil
	}
	list, err := s.loadCodes()
	if err != nil {
		return nil, err
	}
	if s.freezeUniverse {
		s.universe = append([]model.Code(nil), list...)
		s.universeReady = true
	}
	return list, nil
}

func (s *Service) loadCodes() ([]model.Code, error) {
	if s.cfg.ListCodes == nil {
		return nil, fmt.Errorf("pull: 标准证券自动发现需要 ListCodes")
	}
	return s.cfg.ListCodes(s.Context())
}

// ExPool 返回扩展行情(7727)连接池；未配置返回 nil。
func (s *Service) ExPool() tdx.ClientPool { return s.cfg.ExPool }

// Update 执行一次拉取：市场当地收盘后延迟 5 分钟，每个市场每天成功一次。
// must=true 可提前拉取，仍保留市场级去重；提前拉取不会阻止收盘后的更新。
func (s *Service) Update(must ...bool) error { return s.UpdateContext(context.Background(), must...) }

// UpdateContext performs one bounded synchronization; it never owns the injected resources.
func (s *Service) UpdateContext(ctx context.Context, must ...bool) error {
	return s.UpdateWithOptions(ctx, UpdateOptions{AllowEarly: len(must) > 0 && must[0]})
}

// UpdateWithOptions performs a bounded synchronization with independent early
// execution and recheck controls. Context cancellation also applies while queued.
func (s *Service) UpdateWithOptions(ctx context.Context, options UpdateOptions) error {
	if ctx == nil {
		return fmt.Errorf("pull: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.updateGate == nil {
		return fmt.Errorf("pull: service 未初始化")
	}
	select {
	case s.updateGate <- struct{}{}:
		defer func() { <-s.updateGate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.contextMu.Lock()
	s.ctx = ctx
	s.contextMu.Unlock()
	defer func() { s.contextMu.Lock(); s.ctx = nil; s.contextMu.Unlock() }()
	return s.updateWithOptions(time.Now(), options)
}

func (s *Service) update(now time.Time, must bool) error {
	return s.updateWithOptions(now, UpdateOptions{AllowEarly: must})
}

func (s *Service) updateWithOptions(now time.Time, options UpdateOptions) error {
	if err := s.checkOpen(); err != nil {
		return err
	}
	s.discoveryMu.Lock()
	s.discovery = &instrumentDiscovery{}
	s.discoveryMu.Unlock()
	defer func() {
		s.discoveryMu.Lock()
		s.discovery = nil
		s.discoveryMu.Unlock()
	}()

	s.universeMu.Lock()
	s.freezeUniverse = true
	s.universeReady = false
	s.universe = nil
	s.universeMu.Unlock()
	defer func() {
		s.universeMu.Lock()
		s.freezeUniverse = false
		s.universeReady = false
		s.universe = nil
		s.universeMu.Unlock()
	}()
	var firstErr error
	for _, u := range s.units {
		if err := s.Context().Err(); err != nil {
			return err
		}
		schedule, err := s.schedule(u)
		if err != nil {
			s.emit(Event{Market: u.Name(), Status: StatusFailed, Err: err})
			return err
		}
		cutoff, err := schedule.boundary(now)
		if err != nil {
			s.emit(Event{Market: u.Name(), Status: StatusFailed, Err: err})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if now.Before(cutoff) {
			if !options.AllowEarly {
				s.emit(Event{Market: u.Name(), Status: StatusSkipped, Reason: SkipBeforeClose})
				continue
			}
			cutoff = cutoff.AddDate(0, 0, -1)
		}
		key := s.updateKey("pull:" + u.Name())
		unix, has, err := s.store.LoadUpdate(key)
		if err != nil {
			s.emit(Event{Market: u.Name(), Status: StatusFailed, Err: err})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if !options.Recheck && has && unix >= cutoff.Unix() {
			s.emit(Event{Market: u.Name(), Status: StatusSkipped, Reason: SkipCompleted})
			continue
		}
		if err := s.updateUnit(u); err != nil {
			if errors.Is(err, errEmptyUniverse) {
				s.emit(Event{Market: u.Name(), Status: StatusSkipped, Reason: SkipEmpty})
				continue
			}
			s.emit(Event{Market: u.Name(), Status: StatusFailed, Err: err})
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := s.Context().Err(); err != nil {
			s.emit(Event{Market: u.Name(), Status: StatusFailed, Err: err})
			return err
		}
		// Certify only the window selected at the start of this update. Finishing
		// after a later boundary must not mark that later window as processed.
		if err := s.store.SaveUpdate(key, cutoff.Unix()); err != nil {
			s.emit(Event{Market: u.Name(), Status: StatusFailed, Err: err})
			if firstErr == nil {
				firstErr = err
			}
		} else {
			s.emit(Event{Market: u.Name(), Status: StatusSuccess})
		}
	}
	if err := s.Context().Err(); err != nil {
		return err
	}
	return firstErr
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
	schedules := make(map[protocol.Market]Schedule, len(s.cfg.Schedules))
	for market, schedule := range s.cfg.Schedules {
		schedules[market] = schedule
	}
	for _, u := range s.units {
		if _, known := protocol.Market(u.Name()).Info(); !known {
			if schedule, err := s.schedule(u); err == nil {
				schedules[protocol.Market(u.Name())] = schedule
			}
		}
	}
	// 配置仅含 JSON 支持的标量、切片和 map。
	data, _ := json.Marshal(struct {
		Dir, StartAt               string
		Day, Minute, TradeFallback bool
		Codes, Units               []string
		UniverseVersion            string                       `json:",omitempty"`
		Schedules                  map[protocol.Market]Schedule `json:",omitempty"`
	}{s.store.Identity(), s.cfg.StartAt, s.day, s.minute, s.cfg.TradeFallback, codes, units, s.cfg.UniverseVersion, schedules})
	return fmt.Sprintf("%s:%x", prefix, sha256.Sum256(data))
}

var errEmptyUniverse = errors.New("pull: empty universe")

// updateUnit 拉取单个市场；空发现或失败不标记完成。
func (s *Service) updateUnit(u Unit) error {
	codes, err := s.codes(u)
	if err != nil {
		return err
	}
	if err := s.Context().Err(); err != nil {
		return err
	}
	if len(codes) == 0 {
		return errEmptyUniverse
	}
	var tasks sync.WaitGroup
	limit := make(chan struct{}, max(1, s.goro))
	var succCount int64
	var mu sync.Mutex
	var firstFail error
	var completed int
	for _, code := range codes {
		if err := s.Context().Err(); err != nil {
			tasks.Wait()
			return err
		}
		select {
		case limit <- struct{}{}:
		case <-s.Context().Done():
			tasks.Wait()
			return s.Context().Err()
		}
		code := code
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			defer func() { <-limit }()
			fetchWithRetry := func(stage Stage, fetch func(*Service, model.Code) error) (err error) {
				var result FetchResult
				var attempts int
				defer func() {
					status, reason := stageStatus(result, err)
					mu.Lock()
					defer mu.Unlock()
					s.emit(Event{Market: u.Name(), Code: code, Stage: stage, Status: status,
						Reason: reason, Completed: completed, Total: len(codes), Attempts: attempts, Result: result, Err: err})
				}()
				for attempt := 0; attempt < s.retry; attempt++ {
					if err = s.Context().Err(); err != nil {
						return err
					}
					attempts++
					if unit, ok := u.(ResultUnit); ok && s.cfg.OnEvent != nil {
						if stage == StageDay {
							result, err = unit.FetchDayResult(s, code)
						} else {
							result, err = unit.FetchMinuteResult(s, code)
						}
					} else {
						err = fetch(s, code)
					}
					if err == nil {
						err = s.Context().Err()
					}
					if err == nil {
						return nil
					}
					if !retryable(err) || attempt+1 == s.retry {
						return err
					}
					if waitErr := s.retryWait(s.Context(), attempt); waitErr != nil {
						return waitErr
					}
				}
				return err
			}
			var err error
			if s.day {
				err = fetchWithRetry(StageDay, u.FetchDay)
			}
			if err == nil && s.minute {
				err = fetchWithRetry(StageMinute, u.FetchMinute)
			} else if err != nil && s.minute {
				mu.Lock()
				s.emit(Event{Market: u.Name(), Code: code, Stage: StageMinute, Status: StatusSkipped,
					Reason: SkipDayFailed, Completed: completed, Total: len(codes)})
				mu.Unlock()
			}
			if err == nil {
				atomic.AddInt64(&succCount, 1)
			}
			mu.Lock()
			if firstFail == nil && err != nil {
				firstFail = err
			}
			completed++
			if callback := s.cfg.OnProgress; callback != nil {
				callback(Progress{Market: u.Name(), Code: code, Completed: completed, Total: len(codes), Err: err})
			}
			mu.Unlock()
		}()
	}
	tasks.Wait()
	if n := atomic.LoadInt64(&succCount); n < int64(len(codes)) {
		return fmt.Errorf("pull: 市场 %s 拉取不完整（成功 %d/%d），首个错误: %w",
			u.Name(), n, len(codes), firstFail)
	}
	return nil
}

// codes 获取市场代码列表；Config.Codes 指定时优先使用（已按市场分组）。
// 注意：overrides 只包含用户显式列出的市场；其他市场仍走自动发现。
func (s *Service) codes(u Unit) ([]model.Code, error) {
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
func uniqueCodes(codes []model.Code) []model.Code {
	out := make([]model.Code, 0, len(codes))
	seen := make(map[model.Code]bool, len(codes))
	for _, code := range codes {
		key := model.Code{Market: code.Market, Code: code.Code}
		if !seen[key] {
			seen[key] = true
			out = append(out, code)
		}
	}
	return out
}

// 存储相关快捷方法（供 Unit 实现使用）。
