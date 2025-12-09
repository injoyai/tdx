package tdx

import (
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
	"xorm.io/xorm"
)

type IEquity interface {
	Get(code string, t time.Time) *protocol.Equity
	Turnover(code string, t time.Time, volume int64) float64
}

type (
	EquityOption   func(e *Equity)
	DialEquityFunc func(c *Client) (IEquity, error)
)

func WithEquityRetry(retry int) EquityOption {
	return func(s *Equity) {
		s.retry = retry
	}
}

func WithEquitySpec(spec string) EquityOption {
	return func(s *Equity) {
		s.spec = spec
	}
}

func WithEquityDB(db *xorms.Engine) EquityOption {
	return func(s *Equity) {
		s.db = db
	}
}

func WithEquityDialDB(dial func() (*xorms.Engine, error)) EquityOption {
	return func(s *Equity) {
		s.dialDB = dial
	}
}

func WithEquityClient(c *Client) EquityOption {
	return func(s *Equity) {
		s.c = c
	}
}

func WithEquityDialClient(dial DialClientFunc) EquityOption {
	return func(s *Equity) {
		s.dialClient = dial
	}
}

func WithEquityOption(op ...EquityOption) EquityOption {
	return func(s *Equity) {
		for _, o := range op {
			if o != nil {
				o(s)
			}
		}
	}
}

func NewEquity(op ...EquityOption) (*Equity, error) {
	s := &Equity{
		spec:      DefaultEquitySpec,
		retry:     DefaultRetry,
		updateKey: "equity",
		dialDB:    nil,
		m:         make(map[string][]*protocol.Equity),
	}

	WithEquityOption(op...)(s)

	var err error

	//初始化客户端
	if s.c == nil {
		if s.dialClient == nil {
			s.dialClient = func() (*Client, error) { return DialDefault() }
		}
		s.c, err = s.dialClient()
		if err != nil {
			return nil, err
		}
	}

	// 初始化数据库
	if s.db == nil {
		if s.dialDB == nil {
			s.dialDB = func() (*xorms.Engine, error) {
				return xorms.NewSqlite(filepath.Join(DefaultDatabaseDir, "equity.db"))
			}
		}
		s.db, err = s.dialDB()
		if err != nil {
			return nil, err
		}
	}
	if err = s.db.Sync2(new(protocol.Equity)); err != nil {
		return nil, err
	}
	s.updated, err = NewUpdated(s.updateKey, s.db.Engine)
	if err != nil {
		return nil, err
	}

	// 定时/立即更新
	err = NewTimer(s.spec, s.retry, s)

	return s, err
}

type Equity struct {
	spec       string
	retry      int
	updateKey  string
	dialDB     DialDBFunc
	dialClient DialClientFunc

	c       *Client
	db      *xorms.Engine
	updated *Updated
	m       map[string][]*protocol.Equity
	mu      sync.RWMutex
}

func (this *Equity) All() map[string][]*protocol.Equity {
	m := make(map[string][]*protocol.Equity)
	this.mu.RLock()
	defer this.mu.RUnlock()
	for k, v := range this.m {
		m[k] = v
	}
	return m
}

func (this *Equity) Get(code string, t time.Time) *protocol.Equity {
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	for _, v := range ls {
		//读取过来的是15:00,但是今天就生效了,把小时归零,方便判断
		if t.Unix() >= IntegerDay(v.Time).Unix() {
			return v
		}
	}
	return nil
}

func (this *Equity) Turnover(code string, t time.Time, volume int64) float64 {
	x := this.Get(code, t)
	if x == nil {
		return 0
	}
	return x.Turnover(volume)
}

func (this *Equity) Update() error {
	old, err := this.loading()
	if err != nil {
		return err
	}

	this.sort(old)
	this.mu.Lock()
	this.m = old
	this.mu.Unlock()

	updated, err := this.updated.Updated()
	if err == nil && updated {
		return nil
	}
	_new, err := this.update()
	if err != nil {
		return err
	}

	this.sort(_new)
	this.mu.Lock()
	this.m = _new
	this.mu.Unlock()

	return nil
}

func (this *Equity) sort(m map[string][]*protocol.Equity) {
	for _, v := range m {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.After(v[j].Time)
		})
	}
}

func (this *Equity) loading() (map[string][]*protocol.Equity, error) {
	list := []*protocol.Equity(nil)
	if err := this.db.Desc("Time").Find(&list); err != nil {
		return nil, err
	}
	m := map[string][]*protocol.Equity{}
	for _, v := range list {
		m[v.Code] = append(m[v.Code], v)
	}
	return m, nil
}

func (this *Equity) update() (map[string][]*protocol.Equity, error) {
	gbbqs, err := this.c.GetGbbqAll()
	if err != nil {
		return nil, err
	}

	m := gbbqs.GetEquities()
	err = this.db.SessionFunc(func(session *xorm.Session) error {
		if _, err = session.Where("1=1").Delete(new(protocol.Equity)); err != nil {
			return err
		}
		for _, ls := range m {
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
	err = this.updated.Update()
	return m, err
}
