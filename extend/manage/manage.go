// Package manage 组合标准行情连接池、元数据缓存和工作日任务，并管理自建资源的生命周期。
package manage

import (
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/protocol"
	"github.com/robfig/cron/v3"
)

// DefaultClients 是管理器默认的连接数量。
const DefaultClients = 1

// New 创建标准行情管理器；默认初始化连接池、代码缓存和交易日历，股本缓存按需初始化。
func New(op ...Option) (m *Manage, err error) {

	empty := &gbbq.Gbbq{}
	m = &Manage{
		poolClients: DefaultClients,
		Gbbq:        empty, //默认不初始化股本变迁数据
		emptyGbbq:   empty,
	}

	for _, v := range op {
		if v != nil {
			v(m)
		}
	}

	owner := m
	defer func() {
		if err != nil {
			err = errors.Join(err, owner.Close())
		}
	}()

	//连接池
	if isNil(m.ClientPool) {
		if m.dialPool == nil {
			m.dialPool = func() (tdx.ClientPool, error) {
				return tdx.NewPool(func() (tdx.Client, error) { return tdx.DialDefault() }, m.poolClients)
			}
		}
		m.ClientPool, err = m.dialPool()
		m.own(m.ClientPool)
		if err != nil {
			return nil, err
		}
	}
	if isNil(m.ClientPool) {
		return nil, errors.New("连接池工厂返回空对象")
	}

	// 已注入全部组件时，无需等待共享池中的连接。
	if !isNil(m.Codes) && m.Workday != nil && !isNil(m.Gbbq) {
		return m, nil
	}

	//取出一个客户端
	c, err := m.ClientPool.Get()
	if err != nil {
		return nil, err
	}
	if isNil(c) {
		return nil, errors.New("连接池返回空客户端")
	}
	defer m.ClientPool.Put(c)

	//代码管理
	if isNil(m.Codes) {
		if m.dialCodes == nil {
			m.dialCodes = func(c tdx.Client) (codes.Interface, error) { return codes.New(codes.WithClient(c)) }
		}
		m.Codes, err = m.dialCodes(c)
		m.own(m.Codes)
		if err != nil {
			return nil, err
		}
		if isNil(m.Codes) {
			return nil, errors.New("代码管理工厂返回空对象")
		}
	}

	//工作日管理
	if isNil(m.Workday) {
		if m.dialWorkday == nil {
			m.dialWorkday = func(c tdx.Client) (*workday.Workday, error) { return workday.New(workday.WithClient(c)) }
		}
		m.Workday, err = m.dialWorkday(c)
		m.own(m.Workday)
		if err != nil {
			return nil, err
		}
		if isNil(m.Workday) {
			return nil, errors.New("工作日管理工厂返回空对象")
		}
	}

	//股本管理
	if isNil(m.Gbbq) {
		if m.dialGbbq == nil {
			m.dialGbbq = func(c tdx.Client) (gbbq.Interface, error) { return gbbq.New(gbbq.WithClient(c)) }
		}
		m.Gbbq, err = m.dialGbbq(c)
		m.own(m.Gbbq)
		if err != nil {
			return nil, err
		}
		if isNil(m.Gbbq) {
			return nil, errors.New("股本管理工厂返回空对象")
		}
	}

	return
}

type Option func(m *Manage)

func WithClients(clients int) Option {
	return func(m *Manage) {
		m.poolClients = clients
	}
}

func WithPool(pool tdx.ClientPool) Option {
	return func(m *Manage) {
		m.ClientPool = pool
	}
}

func WithDialPool(dial tdx.DialPoolFunc) Option {
	return func(m *Manage) {
		m.ClientPool = nil
		m.dialPool = dial
	}
}

func WithCodes(cache codes.Interface) Option {
	return func(m *Manage) {
		m.Codes = cache
	}
}

func WithDialCodes(dial codes.DialFunc) Option {
	return func(m *Manage) {
		m.Codes = nil
		m.dialCodes = dial
	}
}

func WithWorkday(w *workday.Workday) Option {
	return func(m *Manage) {
		m.Workday = w
	}
}

func WithDialWorkday(dial workday.DialFunc) Option {
	return func(m *Manage) {
		m.Workday = nil
		m.dialWorkday = dial
	}
}

func WithGbbq(g gbbq.Interface) Option {
	return func(m *Manage) {
		m.Gbbq = g
	}
}

func WithDialGbbq(dial gbbq.DialFunc) Option {
	return func(m *Manage) {
		m.Gbbq = nil
		m.dialGbbq = dial
	}
}

func WithDialGbbqDefault() Option {
	return WithDialGbbq(func(c tdx.Client) (gbbq.Interface, error) {
		return gbbq.New(gbbq.WithClient(c))
	})
}

func WithOptions(op ...Option) Option {
	return func(m *Manage) {
		for _, v := range op {
			if v != nil {
				v(m)
			}
		}
	}
}

// Manage 组合标准行情连接源、元数据缓存和工作日任务。使用完毕后调用 Close。
type Manage struct {
	poolClients int
	emptyGbbq   *gbbq.Gbbq // 只允许替换管理器自己创建的股本占位对象。
	dialPool    tdx.DialPoolFunc
	dialCodes   codes.DialFunc
	dialWorkday workday.DialFunc
	dialGbbq    gbbq.DialFunc

	tdx.ClientPool `json:"IPool"`
	Codes          codes.Interface
	Workday        *workday.Workday
	Gbbq           gbbq.Interface

	cron      *cron.Cron
	mu        sync.Mutex
	closed    bool
	owned     []io.Closer
	closeOnce sync.Once
	closeErr  error
}

// RangeStocks 遍历所有股票
func (mgr *Manage) RangeStocks(f func(code string)) {
	for _, v := range mgr.Codes.GetStocks() {
		f(v.FullCode())
	}
}

// RangeETFs 遍历所有ETF
func (mgr *Manage) RangeETFs(f func(code string)) {
	for _, v := range mgr.Codes.GetETFs() {
		f(v.FullCode())
	}
}

// RangeIndexes 遍历所有指数
func (mgr *Manage) RangeIndexes(f func(code string)) {
	for _, v := range mgr.Codes.GetIndexes() {
		f(v.FullCode())
	}
}

// AddWorkdayTask 添加工作日任务。
// 任务内需关闭管理器时，通知外部调用方执行 Close；不得同步调用 Close 等待自身退出。
func (mgr *Manage) AddWorkdayTask(spec string, f func(m *Manage)) error {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if mgr.closed {
		return net.ErrClosed
	}
	if mgr.cron == nil {
		mgr.cron = cron.New(cron.WithSeconds(), cron.WithLocation(protocol.Location()))
		mgr.cron.Start()
	}
	_, err := mgr.cron.AddFunc(spec, func() {
		if mgr.Workday.TodayIs() {
			f(mgr)
		}
	})
	return err
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	switch value := reflect.ValueOf(v); value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}

func (mgr *Manage) own(v any) {
	if c, ok := v.(io.Closer); ok && !isNil(c) {
		mgr.owned = append(mgr.owned, c)
	}
}

// InitGbbq 仅初始化管理器默认的股本占位对象；显式注入或工厂返回的对象保持不变。
// 自建资源由 Manage.Close 释放。
func (mgr *Manage) InitGbbq() (gbbq.Interface, error) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	if mgr.closed {
		return nil, net.ErrClosed
	}
	if g, ok := mgr.Gbbq.(*gbbq.Gbbq); ok && mgr.emptyGbbq != nil && g == mgr.emptyGbbq && g.IsEmpty() {
		g, err := gbbq.New()
		if err != nil {
			return nil, fmt.Errorf("初始化股本变迁数据失败: %w", err)
		}
		mgr.Gbbq = g
		mgr.own(g)
	}
	return mgr.Gbbq, nil
}

// Close 停止工作日任务，并关闭本管理器创建的缓存和连接池；外部注入资源仍由调用方管理。
// 等待在途任务退出，必须从工作日任务外调用。
func (mgr *Manage) Close() error {
	mgr.closeOnce.Do(func() {
		mgr.mu.Lock()
		mgr.closed = true
		cr, owned := mgr.cron, mgr.owned
		mgr.mu.Unlock()
		var done <-chan struct{}
		if cr != nil {
			done = cr.Stop().Done()
		}
		for i := len(owned) - 1; i >= 0; i-- {
			mgr.closeErr = errors.Join(mgr.closeErr, owned[i].Close())
		}
		// 先释放连接池，唤醒任务中等待 Get 的调用。
		if done != nil {
			<-done
		}
	})
	return mgr.closeErr
}
