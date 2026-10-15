package tdx

import (
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"sync"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/robfig/cron/v3"
)

const (
	DefaultClients     = 1
	DefaultRetry       = 3
	DefaultDataDir     = "./output/data"
	DefaultDatabaseDir = DefaultDataDir + "/database"
	DefaultCodesSpec   = "0 1 9 * * *"
	DefaultWorkdaySpec = "0 3 9 * * *"
	DefaultGbbqSpec    = "0 5 9 * * *"
)

func NewManage(op ...Option) (m *Manage, err error) {

	m = &Manage{
		poolClients: DefaultClients,
		Gbbq:        &Gbbq{}, //默认不初始化股本变迁数据
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
	if m.IPool == nil {
		if m.dialPool == nil {
			m.dialPool = func() (IPool, error) {
				return NewPool(func() (Client, error) { return DialDefault() }, m.poolClients)
			}
		}
		m.IPool, err = m.dialPool()
		m.own(m.IPool)
		if err != nil {
			return nil, err
		}
	}

	//取出一个客户端
	c, err := m.IPool.Get()
	if err != nil {
		return nil, err
	}
	defer m.IPool.Put(c)

	//代码管理
	if m.Codes == nil {
		if m.dialCodes == nil {
			m.dialCodes = func(c Client) (ICodes, error) { return NewCodes(WithCodesClient(c)) }
		}
		m.Codes, err = m.dialCodes(c)
		m.own(m.Codes)
		if err != nil {
			return nil, err
		}
	}

	//工作日管理
	if m.Workday == nil {
		if m.dialWorkday == nil {
			m.dialWorkday = func(c Client) (*Workday, error) { return NewWorkday(WithWorkdayClient(c)) }
		}
		m.Workday, err = m.dialWorkday(c)
		m.own(m.Workday)
		if err != nil {
			return nil, err
		}
	}

	//股本管理
	if m.Gbbq == nil {
		if m.dialGbbq == nil {
			m.dialGbbq = func(c Client) (IGbbq, error) { return NewGbbq(WithGbbqClient(c)) }
		}
		m.Gbbq, err = m.dialGbbq(c)
		m.own(m.Gbbq)
		if err != nil {
			return nil, err
		}
	}

	return
}

/*



 */

type (
	Option func(m *Manage)

	DialDBFunc func() (*xorms.Engine, error)

	DialClientFunc func() (Client, error)
)

func WithClients(clients int) Option {
	return func(m *Manage) {
		m.poolClients = clients
	}
}

func WithPool(pool IPool) Option {
	return func(m *Manage) {
		m.IPool = pool
	}
}

func WithDialPool(dial DialPoolFunc) Option {
	return func(m *Manage) {
		m.IPool = nil
		m.dialPool = dial
	}
}

func WithCodes(codes ICodes) Option {
	return func(m *Manage) {
		m.Codes = codes
	}
}

func WithDialCodes(dial DialCodesFunc) Option {
	return func(m *Manage) {
		m.Codes = nil
		m.dialCodes = dial
	}
}

func WithWorkday(w *Workday) Option {
	return func(m *Manage) {
		m.Workday = w
	}
}

func WithDialWorkday(dial DialWorkdayFunc) Option {
	return func(m *Manage) {
		m.Workday = nil
		m.dialWorkday = dial
	}
}

func WithGbbq(gbbq IGbbq) Option {
	return func(m *Manage) {
		m.Gbbq = gbbq
	}
}

func WithDialGbbq(dial DialGbbqFunc) Option {
	return func(m *Manage) {
		m.Gbbq = nil
		m.dialGbbq = dial
	}
}

func WithDialGbbqDefault() Option {
	return WithDialGbbq(func(c Client) (IGbbq, error) {
		return NewGbbq(WithGbbqClient(c))
	})
}

func WithOption(op ...Option) Option {
	return func(m *Manage) {
		for _, v := range op {
			if v != nil {
				v(m)
			}
		}
	}
}

type Manage struct {
	poolClients int
	dialPool    DialPoolFunc
	dialCodes   DialCodesFunc
	dialWorkday DialWorkdayFunc
	dialGbbq    DialGbbqFunc

	IPool
	Codes   ICodes
	Workday *Workday
	Gbbq    IGbbq

	/*

	 */

	cron      *cron.Cron
	mu        sync.Mutex
	closed    bool
	owned     []io.Closer
	closeOnce sync.Once
	closeErr  error
}

// RangeStocks 遍历所有股票
func (this *Manage) RangeStocks(f func(code string)) {
	for _, v := range this.Codes.GetStocks() {
		f(v.FullCode())
	}
}

// RangeETFs 遍历所有ETF
func (this *Manage) RangeETFs(f func(code string)) {
	for _, v := range this.Codes.GetETFs() {
		f(v.FullCode())
	}
}

// RangeIndexes 遍历所有指数
func (this *Manage) RangeIndexes(f func(code string)) {
	for _, v := range this.Codes.GetIndexes() {
		f(v.FullCode())
	}
}

// AddWorkdayTask 添加工作日任务
func (this *Manage) AddWorkdayTask(spec string, f func(m *Manage)) error {
	this.mu.Lock()
	defer this.mu.Unlock()
	if this.closed {
		return net.ErrClosed
	}
	if this.cron == nil {
		this.cron = cron.New(cron.WithSeconds())
		this.cron.Start()
	}
	_, err := this.cron.AddFunc(spec, func() {
		if this.Workday.TodayIs() {
			f(this)
		}
	})
	return err
}

func (m *Manage) own(v any) {
	if c, ok := v.(io.Closer); ok {
		if value := reflect.ValueOf(c); value.Kind() == reflect.Pointer && value.IsNil() {
			return
		}
		m.owned = append(m.owned, c)
	}
}

// InitGbbq 仅在股本数据仍为空实现时初始化；自建资源由 Manage.Close 释放。
func (m *Manage) InitGbbq() (IGbbq, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, net.ErrClosed
	}
	if g, ok := m.Gbbq.(*Gbbq); ok && g.IsEmpty() {
		g, err := NewGbbq()
		if err != nil {
			return nil, fmt.Errorf("初始化股本变迁数据失败: %w", err)
		}
		m.Gbbq = g
		m.own(g)
	}
	return m.Gbbq, nil
}

// Close 停止工作日任务，并关闭本管理器创建的缓存和连接池；外部注入资源仍由调用方管理。
func (m *Manage) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		cr, owned := m.cron, m.owned
		m.mu.Unlock()
		var done <-chan struct{}
		if cr != nil {
			done = cr.Stop().Done()
		}
		for i := len(owned) - 1; i >= 0; i-- {
			m.closeErr = errors.Join(m.closeErr, owned[i].Close())
		}
		// 先释放连接池，唤醒任务中等待 Get 的调用。
		if done != nil {
			<-done
		}
	})
	return m.closeErr
}
