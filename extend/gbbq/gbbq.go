// Package gbbq 提供股本、除权除息记录的持久化缓存和复权查询。
package gbbq

import (
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/injoyai/tdx/v2"
	query "github.com/injoyai/tdx/v2/internal/gbbq"
	"github.com/injoyai/tdx/v2/internal/lifecycle"
	"github.com/injoyai/tdx/v2/internal/metadata"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

// DefaultSpec 是股本缓存每日刷新的默认定时规则。
const DefaultSpec = "0 5 9 * * *"

// Interface 提供股本、换手率和复权因子的只读查询。
type Interface interface {
	GetEquity(code string, t time.Time) *protocol.Equity
	GetTurnover(code string, t time.Time, volume int64) float64
	GetXRXDs(code string) protocol.XRXDs
	GetFactors(code string, ks protocol.Klines) []*protocol.Factor
}

type (
	Option func(e *Gbbq)

	DialFunc func(c tdx.Client) (Interface, error)
)

func WithRetry(retry int) Option {
	return func(s *Gbbq) {
		s.retry = retry
	}
}

func WithSpec(spec string) Option {
	return func(s *Gbbq) {
		s.spec = spec
	}
}

func WithDB(db *xorms.Engine) Option {
	return func(s *Gbbq) {
		s.db = db
	}
}

func WithDialDB(dial xorms.DialFunc) Option {
	return func(s *Gbbq) {
		s.dialDB = dial
	}
}

func WithClient(c tdx.Client) Option {
	return func(s *Gbbq) {
		s.c = clientOrNil(c)
	}
}

func WithDialClient(dial tdx.DialClientFunc) Option {
	return func(s *Gbbq) {
		s.dialClient = dial
	}
}

func WithOptions(op ...Option) Option {
	return func(s *Gbbq) {
		for _, o := range op {
			if o != nil {
				o(s)
			}
		}
	}
}

// New 创建并立即刷新股本缓存，再启动定时更新；自建资源由 Close 释放。
func New(op ...Option) (result *Gbbq, err error) {
	s := &Gbbq{
		spec:      DefaultSpec,
		retry:     tdx.DefaultRetry,
		updateKey: "gbbq",
		dialDB:    nil,
		m:         make(map[string][]*protocol.Gbbq),
	}

	WithOptions(op...)(s)

	ownedClient, ownedDB := s.c == nil, s.db == nil
	s.cleanup = func() error {
		var err error
		if ownedClient && clientOrNil(s.c) != nil {
			err = errors.Join(err, s.c.Close())
		}
		if ownedDB && s.db != nil && s.db.Engine != nil {
			err = errors.Join(err, s.db.Close())
		}
		return err
	}
	ready := false
	defer func() {
		if !ready {
			err = errors.Join(err, s.Close())
		}
	}()

	//初始化客户端
	if s.c == nil {
		if s.dialClient == nil {
			s.dialClient = func() (tdx.Client, error) { return tdx.DialDefault() }
		}
		s.c, err = s.dialClient()
		if err != nil {
			return nil, err
		}
		s.c = clientOrNil(s.c)
		if s.c == nil {
			return nil, errors.New("client is nil")
		}
	}

	// 初始化数据库
	if s.db == nil {
		if s.dialDB == nil {
			s.dialDB = func() (*xorms.Engine, error) {
				return xorms.NewSQLite(filepath.Join(metadata.DefaultDatabaseDir, "gbbq.db"))
			}
		}
		s.db, err = s.dialDB()
		if err != nil {
			return nil, err
		}
	}
	if s.db == nil || s.db.Engine == nil {
		return nil, errors.New("数据库工厂返回空引擎")
	}
	if err = s.db.Sync2(new(protocol.Gbbq)); err != nil {
		return nil, err
	}
	s.updated, err = update.New(s.db, 9, 0)
	if err != nil {
		return nil, err
	}

	// 定时/立即更新
	s.stop, err = update.StartTimer(s.spec, s.retry, s)
	if err != nil {
		return nil, err
	}
	ready = true
	return s, nil
}

type Gbbq struct {
	state      lifecycle.Updates
	stop       func()
	cleanup    func() error
	spec       string
	retry      int
	updateKey  string
	dialDB     xorms.DialFunc
	dialClient tdx.DialClientFunc

	c       tdx.Client
	db      *xorms.Engine
	updated *update.Tracker
	m       map[string][]*protocol.Gbbq
	mu      sync.RWMutex
}

// IsEmpty 判断是否为未初始化的空实现（manage.New 默认塞入的 &Gbbq{}）。
// 空实现的 GetEquity/GetTurnover 等恒返回零值，与"已初始化但无数据"不同。
func (g *Gbbq) IsEmpty() bool {
	if g == nil {
		return true
	}
	return g.db == nil && g.c == nil && g.m == nil
}

func (g *Gbbq) All() map[string][]*protocol.Gbbq {
	m := make(map[string][]*protocol.Gbbq)
	g.mu.RLock()
	defer g.mu.RUnlock()
	for k, v := range g.m {
		m[k] = v
	}
	return m
}

func (g *Gbbq) records(code string) []*protocol.Gbbq {
	key, err := metadata.CodeKey(code)
	if err != nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.m[key]
}

func (g *Gbbq) GetEquity(code string, t time.Time) *protocol.Equity {
	return query.EquityAt(g.records(code), t)
}

func (g *Gbbq) GetXRXDs(code string) protocol.XRXDs {
	return query.XRXDs(g.records(code))
}

func (g *Gbbq) GetXRXDMap(code string) map[string]*protocol.XRXD {
	ls := g.records(code)
	res := map[string]*protocol.XRXD{}
	for _, v := range ls {
		if v.IsXRXD() {
			res[v.Time.Format(time.DateOnly)] = v.XRXD()
		}
	}
	return res
}

func (g *Gbbq) GetFactors(code string, ks protocol.Klines) []*protocol.Factor {
	return g.GetXRXDs(code).Pre(ks).Factors()
}

// QFQ 把已获取的不复权日线 ks 转为前复权日线(对齐通达信桌面端, 四舍五入到分)。
// 已有 K 线时用此方法; 若要一步到位拉取+复权用 QFQKlineDay。
func (g *Gbbq) QFQ(code string, ks protocol.Klines) protocol.Klines {
	return protocol.ApplyQFQ(ks, g.GetFactors(code, ks))
}

// HFQ 把已获取的不复权日线转为后复权日线。见 QFQ。
func (g *Gbbq) HFQ(code string, ks protocol.Klines) protocol.Klines {
	return protocol.ApplyHFQ(ks, g.GetFactors(code, ks))
}

// QFQKlineDay 拉取全量历史并前复权，成交量统一为整数手，可直接用于指标和本地文件。
func (g *Gbbq) QFQKlineDay(code string) (protocol.Klines, error) {
	resp, err := g.c.GetKlineDayAll(code)
	if err != nil {
		return nil, err
	}
	ks, err := resp.Klines()
	if err != nil {
		return nil, err
	}
	return g.QFQ(code, ks), nil
}

// HFQKlineDay 一站式获取后复权日线(全量历史)。见 QFQKlineDay。
func (g *Gbbq) HFQKlineDay(code string) (protocol.Klines, error) {
	resp, err := g.c.GetKlineDayAll(code)
	if err != nil {
		return nil, err
	}
	ks, err := resp.Klines()
	if err != nil {
		return nil, err
	}
	return g.HFQ(code, ks), nil
}

func (g *Gbbq) GetTurnover(code string, t time.Time, volume int64) float64 {
	return query.Turnover(g.GetEquity(code, t), volume)
}

func (g *Gbbq) Update() error {
	return g.state.Run(g.refresh)
}

func (g *Gbbq) refresh() error {
	old, err := g.load()
	if err != nil {
		return err
	}

	g.sort(old)
	g.mu.Lock()
	g.m = old
	g.mu.Unlock()

	updated, err := g.updated.IsUpdated(g.updateKey)
	if err == nil && updated {
		return nil
	}
	fresh, err := g.update()
	if err != nil {
		return err
	}

	g.sort(fresh)
	g.mu.Lock()
	g.m = fresh
	g.mu.Unlock()

	return nil
}

func (*Gbbq) sort(m map[string][]*protocol.Gbbq) {
	for _, v := range m {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.Before(v[j].Time)
		})
	}
}

func (g *Gbbq) load() (map[string][]*protocol.Gbbq, error) {
	list := []*protocol.Gbbq(nil)
	if err := g.db.Asc("Time").Find(&list); err != nil {
		return nil, err
	}
	m := map[string][]*protocol.Gbbq{}
	for _, v := range list {
		m[v.Code] = append(m[v.Code], v)
	}
	return m, nil
}

func (g *Gbbq) update() (map[string][]*protocol.Gbbq, error) {
	gbbqs, err := g.c.GetGbbqAll()
	if err != nil {
		return nil, err
	}
	err = g.db.SessionFunc(func(session *xorm.Session) error {
		if _, err = session.Where("1=1").Delete(new(protocol.Gbbq)); err != nil {
			return err
		}
		for _, ls := range gbbqs {
			for _, v := range ls {
				if _, err = session.Insert(v); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = g.updated.Mark(g.updateKey)
	return gbbqs, err
}

// Close 停止更新并释放自建资源；外部注入的客户端和数据库仍由调用方关闭。
func (g *Gbbq) Close() error { return g.state.Close(g.stop, g.cleanup) }

func clientOrNil(c tdx.Client) tdx.Client {
	if c != nil && reflect.ValueOf(c).Kind() == reflect.Pointer && reflect.ValueOf(c).IsNil() {
		return nil
	}
	return c
}
