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

type IGbbq interface {
	GetEquity(code string, t time.Time) *protocol.Equity
	GetTurnover(code string, t time.Time, volume int64) float64
	GetXRXDs(code string) protocol.XRXDs
	GetFactors(code string, ks protocol.Klines) []*protocol.Factor
}

type (
	GbbqOption func(e *Gbbq)

	DialGbbqFunc func(c *Client) (IGbbq, error)
)

func WithGbbqRetry(retry int) GbbqOption {
	return func(s *Gbbq) {
		s.retry = retry
	}
}

func WithGbbqSpec(spec string) GbbqOption {
	return func(s *Gbbq) {
		s.spec = spec
	}
}

func WithGbbqDB(db *xorms.Engine) GbbqOption {
	return func(s *Gbbq) {
		s.db = db
	}
}

func WithGbbqDialDB(dial func() (*xorms.Engine, error)) GbbqOption {
	return func(s *Gbbq) {
		s.dialDB = dial
	}
}

func WithGbbqClient(c *Client) GbbqOption {
	return func(s *Gbbq) {
		s.c = c
	}
}

func WithGbbqDialClient(dial DialClientFunc) GbbqOption {
	return func(s *Gbbq) {
		s.dialClient = dial
	}
}

func WithGbbqOption(op ...GbbqOption) GbbqOption {
	return func(s *Gbbq) {
		for _, o := range op {
			if o != nil {
				o(s)
			}
		}
	}
}

func NewGbbq(op ...GbbqOption) (*Gbbq, error) {
	s := &Gbbq{
		spec:      DefaultGbbqSpec,
		retry:     DefaultRetry,
		updateKey: "gbbq",
		dialDB:    nil,
		m:         make(map[string][]*protocol.Gbbq),
	}

	WithGbbqOption(op...)(s)

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
				return xorms.NewSqlite(filepath.Join(DefaultDatabaseDir, "gbbq.db"))
			}
		}
		s.db, err = s.dialDB()
		if err != nil {
			return nil, err
		}
	}
	if err = s.db.Sync2(new(protocol.Gbbq)); err != nil {
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

type Gbbq struct {
	spec       string
	retry      int
	updateKey  string
	dialDB     DialDBFunc
	dialClient DialClientFunc

	c       *Client
	db      *xorms.Engine
	updated *Updated
	m       map[string][]*protocol.Gbbq
	mu      sync.RWMutex
}

func (this *Gbbq) All() map[string][]*protocol.Gbbq {
	m := make(map[string][]*protocol.Gbbq)
	this.mu.RLock()
	defer this.mu.RUnlock()
	for k, v := range this.m {
		m[k] = v
	}
	return m
}

func (this *Gbbq) GetEquity(code string, t time.Time) *protocol.Equity {
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	for i := len(ls) - 1; i >= 0; i-- {
		v := ls[i]
		//读取过来的是15:00,但是今天就生效了,把小时归零,方便判断
		if v.IsEquity() && t.Unix() >= IntegerDay(v.Time).Unix() {
			return v.Equity()
		}
	}
	return nil
}

func (this *Gbbq) GetXRXDs(code string) protocol.XRXDs {
	code = protocol.AddPrefix(code)
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	res := protocol.XRXDs{}
	for _, v := range ls {
		if v.IsXRXD() {
			res = append(res, v.XRXD())
		}
	}
	return res
}

func (this *Gbbq) GetXRXDMap(code string) map[string]*protocol.XRXD {
	code = protocol.AddPrefix(code)
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	res := map[string]*protocol.XRXD{}
	for _, v := range ls {
		if v.IsXRXD() {
			res[v.Time.Format(time.DateOnly)] = v.XRXD()
		}
	}
	return res
}

func (this *Gbbq) GetFactors(code string, ks protocol.Klines) []*protocol.Factor {
	return this.GetXRXDs(code).Pre(ks).Factors()
}

func (this *Gbbq) GetTurnover(code string, t time.Time, volume int64) float64 {
	x := this.GetEquity(code, t)
	if x == nil {
		return 0
	}
	return x.Turnover(volume)
}

func (this *Gbbq) Update() error {
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

func (this *Gbbq) sort(m map[string][]*protocol.Gbbq) {
	for _, v := range m {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Time.Before(v[j].Time)
		})
	}
}

func (this *Gbbq) loading() (map[string][]*protocol.Gbbq, error) {
	list := []*protocol.Gbbq(nil)
	if err := this.db.Asc("Time").Find(&list); err != nil {
		return nil, err
	}
	m := map[string][]*protocol.Gbbq{}
	for _, v := range list {
		m[v.Code] = append(m[v.Code], v)
	}
	return m, nil
}

func (this *Gbbq) update() (map[string][]*protocol.Gbbq, error) {
	gbbqs, err := this.c.GetGbbqAll()
	if err != nil {
		return nil, err
	}
	err = this.db.SessionFunc(func(session *xorm.Session) error {
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
	err = this.updated.Update()
	return gbbqs, err
}
