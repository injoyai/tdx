package tdx

import (
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/lib/gbbq"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/robfig/cron/v3"
	"xorm.io/xorm"
)

type IEquity interface {
	Get(code string, t time.Time) *gbbq.Equity
	Turnover(code string, t time.Time, volume float64) float64
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

func WithEquityTempDir(dir string) EquityOption {
	return func(s *Equity) {
		s.tempDir = dir
	}
}

func WithEquityDialDB(dial func() (*xorms.Engine, error)) EquityOption {
	return func(s *Equity) {
		s.dialDB = dial
	}
}

func NewEquity(op ...EquityOption) (*Equity, error) {
	s := &Equity{
		spec:      "0 5 9 * * *",
		retry:     DefaultRetry,
		updateKey: "equity",
		tempDir:   filepath.Join(DefaultDataDir, "temp"),
		dialDB:    nil,
		m:         make(map[string]gbbq.Equities),
	}

	for _, o := range op {
		o(s)
	}

	var err error

	// 初始化数据库
	if s.dialDB == nil {
		s.dialDB = func() (*xorms.Engine, error) {
			return xorms.NewSqlite(filepath.Join(DefaultDatabaseDir, "equity.db"))
		}
	}
	s.db, err = s.dialDB()
	if err != nil {
		return nil, err
	}
	if err = s.db.Sync2(new(gbbq.Equity)); err != nil {
		return nil, err
	}
	s.updated, err = NewUpdated(s.updateKey, s.db.Engine)
	if err != nil {
		return nil, err
	}

	// 立即更新
	err = s.Update()
	if err != nil {
		return nil, err
	}

	// 定时更新
	cr := cron.New(cron.WithSeconds())
	_, err = cr.AddFunc(s.spec, func() {
		for i := 0; i == 0 || i < s.retry; i++ {
			if err := s.Update(); err != nil {
				logs.Err(err)
				<-time.After(time.Minute * 5)
			} else {
				break
			}
		}
	})
	if err != nil {
		return nil, err
	}

	cr.Start()

	return s, nil
}

type Equity struct {
	spec      string
	retry     int
	updateKey string
	tempDir   string
	dialDB    func() (*xorms.Engine, error)

	db      *xorms.Engine
	updated *Updated
	m       map[string]gbbq.Equities
	mu      sync.RWMutex
}

func (this *Equity) Get(code string, t time.Time) *gbbq.Equity {
	if len(code) == 8 {
		code = code[2:]
	}
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	for _, v := range ls {
		if t.Unix() >= v.Date.Unix() {
			return v
		}
	}
	return nil
}

func (this *Equity) Turnover(code string, t time.Time, volume float64) float64 {
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

	cache := old.Map()
	this.sort(cache)
	this.mu.Lock()
	this.m = cache
	this.mu.Unlock()

	updated, err := this.updated.Updated()
	if err == nil && updated {
		return nil
	}
	_new, err := this.update(old)
	if err != nil {
		return err
	}

	this.sort(_new)
	this.mu.Lock()
	this.m = _new
	this.mu.Unlock()

	return nil
}

func (this *Equity) sort(m map[string]gbbq.Equities) {
	for _, v := range m {
		sort.Slice(v, func(i, j int) bool {
			return v[i].Date.After(v[j].Date)
		})
	}
}

func (this *Equity) loading() (gbbq.Equities, error) {
	list := gbbq.Equities(nil)
	if err := this.db.Desc("Date").Find(&list); err != nil {
		return nil, err
	}
	return list, nil
}

func (this *Equity) update(old gbbq.Equities) (map[string]gbbq.Equities, error) {
	ss, err := gbbq.DownloadAndDecode(this.tempDir)
	if err != nil {
		return nil, err
	}

	m := ss.GetStocks()
	insert := make(map[string]*gbbq.Equity)
	for _, v := range m {
		for _, vv := range v {
			insert[vv.Code+vv.Date.Format(time.DateOnly)] = vv
		}
	}

	_delete := gbbq.Equities{}
	for _, v := range old {
		key := v.Code + v.Date.Format(time.DateOnly)
		if _new, ok := insert[key]; !ok {
			_delete = append(_delete, _new)
		}
		delete(insert, key)
	}

	err = this.db.SessionFunc(func(session *xorm.Session) error {
		for _, v := range _delete {
			if _, err := session.Where("Code=? and Date=?", v.Code, v.Date).Delete(v); err != nil {
				return err
			}
		}
		for _, v := range insert {
			if _, err := session.Insert(v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = this.updated.Update()
	return ss.GetStocks(), err
}
