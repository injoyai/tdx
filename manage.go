package tdx

import (
	"sync"

	"github.com/injoyai/tdx/lib/xorms"
	"github.com/robfig/cron/v3"
)

const (
	DefaultClients     = 1
	DefaultRetry       = 3
	DefaultDataDir     = "./data"
	DefaultDatabaseDir = "./data/database"
)

func NewManageMysql(dsn string, op ...Option) (*Manage, error) {
	return NewManage(
		WithDialCodes(func(c *Client) (ICodes, error) {
			return NewCodesMysql(dsn, WithCodesClient(c))
		}),
		WithDialWorkday(func(c *Client) (*Workday, error) {
			return NewWorkdayMysql(dsn, WithWorkdayClient(c))
		}),
		WithOptions(op...),
	)
}

func NewManage(op ...Option) (m *Manage, err error) {

	m = &Manage{poolClients: DefaultClients}

	for _, v := range op {
		if v != nil {
			v(m)
		}
	}

	//连接池
	if m.IPool == nil {
		if m.dialPool == nil {
			m.dialPool = func() (IPool, error) {
				return NewPool(func() (*Client, error) { return DialDefault() }, m.poolClients)
			}
		}
		m.IPool, err = m.dialPool()
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
			m.dialCodes = func(c *Client) (ICodes, error) { return NewCodes(WithCodesClient(c)) }
		}
		m.Codes, err = m.dialCodes(c)
		if err != nil {
			return nil, err
		}
	}

	//工作日管理
	if m.Workday == nil {
		if m.dialWorkday == nil {
			m.dialWorkday = func(c *Client) (*Workday, error) { return NewWorkday(WithWorkdayClient(c)) }
		}
		m.Workday, err = m.dialWorkday(c)
		if err != nil {
			return nil, err
		}
	}

	//股本管理
	if m.Equity == nil {
		if m.dialEquity != nil {
			m.Equity, err = m.dialEquity(c)
			if err != nil {
				return nil, err
			}
		}
	}

	return
}

/*



 */

type (
	Option func(m *Manage)

	DialDBFunc func() (*xorms.Engine, error)

	DialClientFunc func() (*Client, error)
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
		m.dialWorkday = dial
	}
}

func WithEquity(equity IEquity) Option {
	return func(m *Manage) {
		m.Equity = equity
	}
}

func WithDialEquity(dial DialEquityFunc) Option {
	return func(m *Manage) {
		m.dialEquity = dial
	}
}

func WithDialEquityDefault() Option {
	return func(m *Manage) {
		m.dialEquity = func(c *Client) (IEquity, error) { return NewEquity() }
	}
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

type Manage struct {
	poolClients int
	dialPool    DialPoolFunc
	dialCodes   DialCodesFunc
	dialWorkday DialWorkdayFunc
	dialEquity  DialEquityFunc

	IPool
	Codes   ICodes
	Workday *Workday
	Equity  IEquity

	/*

	 */

	cron *cron.Cron
	once sync.Once
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
	for _, v := range this.Codes.GetETFs() {
		f(v.FullCode())
	}
}

// AddWorkdayTask 添加工作日任务
func (this *Manage) AddWorkdayTask(spec string, f func(m *Manage)) error {
	this.once.Do(func() {
		this.cron = cron.New(cron.WithSeconds())
		this.cron.Start()
	})
	_, err := this.cron.AddFunc(spec, func() {
		if this.Workday.TodayIs() {
			f(this)
		}
	})
	return err
}
