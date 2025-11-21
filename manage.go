package tdx

import (
	"errors"
	"sync"

	"github.com/injoyai/conv"
	"github.com/injoyai/ios/client"
	"github.com/robfig/cron/v3"
)

const (
	DefaultClients     = 1
	DefaultDataDir     = "./data"
	DefaultDatabaseDir = "./data/database"
)

func NewManageMysql(op ...Option) (*Manage, error) {
	return NewManage(
		WithOptions(op...),
		WithDialCodes(func(c *Client, database string) (ICodes, error) {
			if database == "" {
				return nil, errors.New("未配置Codes的数据库")
			}
			return NewCodesMysql(c, database)
		}),
		WithDialWorkday(func(c *Client, database string) (*Workday, error) {
			if database == "" {
				return nil, errors.New("未配置Workday的数据库")
			}
			return NewWorkdayMysql(c, database)
		}),
	)
}

func NewManageSqlite(op ...Option) (*Manage, error) {
	return NewManage(
		WithCodesDatabase(DefaultDatabaseDir+"/codes.db"),
		WithWorkdayDatabase(DefaultDatabaseDir+"/workday.db"),
		WithOptions(op...),
		WithDialCodes(func(c *Client, database string) (ICodes, error) {
			return NewCodesSqlite(c, database)
		}),
		WithDialWorkday(func(c *Client, database string) (*Workday, error) {
			return NewWorkdaySqlite(c, database)
		}),
	)
}

func NewManageSqlite2(op ...Option) (*Manage, error) {
	return NewManage(
		WithCodesDatabase(DefaultDatabaseDir+"/codes2.db"),
		WithWorkdayDatabase(DefaultDatabaseDir+"/workday.db"),
		WithOptions(op...),
		WithDialCodes(func(c *Client, database string) (ICodes, error) {
			return NewCodes2(
				WithCodes2Client(c),
				WithCodes2Database(database),
			)
		}),
		WithDialWorkday(func(c *Client, database string) (*Workday, error) {
			return NewWorkdaySqlite(c, database)
		}),
	)

}

func NewManage(op ...Option) (m *Manage, err error) {

	m = &Manage{
		clients:         DefaultClients,
		dial:            DialDefault,
		dialOptions:     nil,
		dialCodes:       nil,
		codesDatabase:   DefaultDatabaseDir + "/codes2.db",
		dialWorkday:     nil,
		workdayDatabase: DefaultDatabaseDir + "/workday.db",
		Pool:            nil,
		Codes:           nil,
		Workday:         nil,
		cron:            nil,
		once:            sync.Once{},
	}

	for _, v := range op {
		if v != nil {
			v(m)
		}
	}

	m.clients = conv.Select(m.clients <= 0, 1, m.clients)
	m.dial = conv.Select(m.dial == nil, DialDefault, m.dial)

	//连接池
	m.Pool, err = NewPool(func() (*Client, error) { return m.dial(m.dialOptions...) }, m.clients)
	if err != nil {
		return nil, err
	}

	//代码管理
	if m.Codes == nil {
		if m.dialCodes == nil {
			m.dialCodes = func(c *Client, database string) (ICodes, error) {
				return NewCodes2(WithCodes2Client(c), WithCodes2Database(database))
			}
		}
		err = m.Pool.Do(func(c *Client) error {
			m.Codes, err = m.dialCodes(c, m.codesDatabase)
			return err
		})
		if err != nil {
			return nil, err
		}
	}

	//工作日管理
	if m.Workday == nil {
		if m.dialWorkday == nil {
			m.dialWorkday = func(c *Client, database string) (*Workday, error) {
				return NewWorkdaySqlite(c, database)
			}
		}
		err = m.Pool.Do(func(c *Client) error {
			m.Workday, err = m.dialWorkday(c, m.workdayDatabase)
			return err
		})
		if err != nil {
			return nil, err
		}
	}

	return
}

/*



 */

type Option func(m *Manage)
type DialWorkdayFunc func(c *Client, database string) (*Workday, error)
type DialCodesFunc func(c *Client, database string) (ICodes, error)

func WithClients(clients int) Option {
	return func(m *Manage) {
		m.clients = clients
	}
}

func WithDial(dial func(op ...client.Option) (*Client, error), op ...client.Option) Option {
	return func(m *Manage) {
		m.dial = dial
		m.dialOptions = op
	}
}

func WithDialOptions(op ...client.Option) Option {
	return func(m *Manage) {
		m.dialOptions = op
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

func WithCodesDatabase(database string) Option {
	return func(m *Manage) {
		m.codesDatabase = database
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

func WithWorkdayDatabase(database string) Option {
	return func(m *Manage) {
		m.workdayDatabase = database
	}
}

func WithOptions(op ...Option) Option {
	return func(m *Manage) {
		for _, v := range op {
			v(m)
		}
	}
}

type Manage struct {
	clients         int
	dial            func(op ...client.Option) (cli *Client, err error)
	dialOptions     []client.Option
	dialCodes       func(c *Client, database string) (ICodes, error)
	codesDatabase   string
	dialWorkday     DialWorkdayFunc
	workdayDatabase string

	/*

	 */

	*Pool
	Codes   ICodes
	Workday *Workday
	cron    *cron.Cron
	once    sync.Once
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
func (this *Manage) AddWorkdayTask(spec string, f func(m *Manage)) {
	this.once.Do(func() {
		this.cron = cron.New(cron.WithSeconds())
		this.cron.Start()
	})
	this.cron.AddFunc(spec, func() {
		if this.Workday.TodayIs() {
			f(this)
		}
	})
}

type ManageConfig struct {
	Number          int                                                //客户端数量
	CodesFilename   string                                             //代码数据库位置
	WorkdayFileName string                                             //工作日数据库位置
	Dial            func(op ...client.Option) (cli *Client, err error) //默认连接方式
}
