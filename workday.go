package tdx

import (
	"errors"
	"iter"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	"github.com/injoyai/base/maps"
	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
	"github.com/robfig/cron/v3"
)

type (
	WorkdayOption   func(w *Workday)
	DialWorkdayFunc func(c *Client) (*Workday, error)
)

func WithWorkdaySpec(spec string) WorkdayOption {
	return func(w *Workday) {
		w.spec = spec
	}
}

func WithWorkdayRetry(retry int) WorkdayOption {
	return func(w *Workday) {
		w.retry = retry
	}
}

func WithWorkdayDB(db *xorms.Engine) WorkdayOption {
	return func(w *Workday) {
		w.db = db
	}
}

func WithWorkdayDialDB(dial DialDBFunc) WorkdayOption {
	return func(w *Workday) {
		w.dialDB = dial
	}
}

func WithWorkdayClient(c *Client) WorkdayOption {
	return func(w *Workday) {
		w.c = c
	}
}

func WithWorkdayDialClient(dial DialClientFunc) WorkdayOption {
	return func(w *Workday) {
		w.dialClient = dial
	}
}

func WithWorkdayOption(op ...WorkdayOption) WorkdayOption {
	return func(w *Workday) {
		for _, v := range op {
			v(w)
		}
	}
}

func NewWorkdayMysql(dsn string, op ...WorkdayOption) (*Workday, error) {
	return NewWorkday(
		WithWorkdayDialDB(func() (*xorms.Engine, error) { return xorms.NewMysql(dsn) }),
		WithWorkdayOption(op...),
	)
}

func NewWorkdaySqlite(op ...WorkdayOption) (*Workday, error) {
	return NewWorkday(op...)
}

func NewWorkday(op ...WorkdayOption) (*Workday, error) {

	w := &Workday{
		spec:       "0 3 9 * * *",
		retry:      DefaultRetry,
		dialDB:     nil,
		dialClient: nil,

		c:     nil,
		db:    nil,
		cache: maps.NewBit(),
	}

	for _, v := range op {
		v(w)
	}

	var err error
	if w.db == nil {
		if w.dialDB == nil {
			w.dialDB = func() (*xorms.Engine, error) {
				return xorms.NewSqlite(filepath.Join(DefaultDatabaseDir, "workday.db"))
			}
		}
		w.db, err = w.dialDB()
		if err != nil {
			return nil, err
		}
	}
	if err := w.db.Sync2(new(WorkdayModel)); err != nil {
		return nil, err
	}

	if w.c == nil {
		if w.dialClient == nil {
			w.dialClient = func() (*Client, error) { return DialDefault() }
		}
		w.c, err = w.dialClient()
		if err != nil {
			return nil, err
		}
	}

	err = w.Update()
	if err != nil {
		return nil, err
	}

	//设置定时器,每天早上9点更新数据,8点多获取不到今天的数据
	task := cron.New(cron.WithSeconds())
	_, err = task.AddFunc(w.spec, func() {
		for i := 0; i == 0 || i < w.retry; i++ {
			err := w.Update()
			if err == nil {
				return
			}
			logs.Err(err)
			<-time.After(time.Minute * 5)
		}
	})
	if err != nil {
		return nil, err
	}

	task.Start()

	return w, nil
}

type Workday struct {
	spec       string
	retry      int
	dialDB     DialDBFunc
	dialClient DialClientFunc

	c     *Client
	db    *xorms.Engine
	cache maps.Bit
}

// Update 更新
func (this *Workday) Update() error {

	if this.c == nil {
		return errors.New("client is nil")
	}

	//获取沪市指数的日K线,用作历史是否节假日的判断依据
	//判断日K线是否拉取过

	//获取全部工作日
	all := []*WorkdayModel(nil)
	if err := this.db.Find(&all); err != nil {
		return err
	}
	var lastWorkday = &WorkdayModel{}
	if len(all) > 0 {
		lastWorkday = all[len(all)-1]
	}
	for _, v := range all {
		this.cache.Set(uint64(v.Unix), true)
	}

	now := time.Now()
	if lastWorkday.Unix < IntegerDay(now).Unix() {
		resp, err := this.c.GetIndexDayAll("sh000001")
		if err != nil {
			logs.Err(err)
			return err
		}

		inserts := []any(nil)
		for _, v := range resp.List {
			if unix := v.Time.Unix(); unix > lastWorkday.Unix {
				inserts = append(inserts, &WorkdayModel{Unix: unix, Date: v.Time.Format("20060102")})
				this.cache.Set(uint64(unix), true)
			}
		}

		if len(inserts) == 0 {
			return nil
		}

		_, err = this.db.Insert(inserts)
		return err

	}

	return nil
}

// Is 是否是工作日
func (this *Workday) Is(t time.Time) bool {
	return this.cache.Get(uint64(IntegerDay(t).Add(time.Hour * 15).Unix()))
}

// TodayIs 今天是否是工作日
func (this *Workday) TodayIs() bool {
	return this.Is(time.Now())
}

// RangeYear 遍历一年的所有工作日
func (this *Workday) RangeYear(year int, f func(t time.Time) bool) {
	this.Range(
		time.Date(year, 1, 1, 0, 0, 0, 0, time.Local),
		time.Date(year, 12, 31, 0, 0, 0, 1, time.Local),
		f,
	)
}

// Range 遍历指定范围的工作日,推荐start带上时间15:00,这样当天小于15点不会触发
func (this *Workday) Range(start, end time.Time, f func(t time.Time) bool) {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	for ; start.Before(end); start = start.Add(time.Hour * 24) {
		if this.Is(start) {
			if !f(start) {
				return
			}
		}
	}
}

func (this *Workday) IterYear(year int, desc ...bool) iter.Seq[time.Time] {
	return this.Iter(
		time.Date(year, 1, 1, 0, 0, 0, 0, time.Local),
		time.Date(year, 12, 31, 0, 0, 0, 1, time.Local),
		desc...,
	)
}

// Iter 遍历指定范围的工作日,推荐start带上时间15:00,这样当天小于15点不会触发
func (this *Workday) Iter(start, end time.Time, desc ...bool) iter.Seq[time.Time] {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	if len(desc) > 0 && desc[0] {
		//倒序遍历
		return func(yield func(time.Time) bool) {
			for ; end.After(start); end = end.Add(-time.Hour * 24) {
				if this.Is(end) {
					if !yield(end) {
						return
					}
				}
			}
		}
	}
	//正序遍历
	return func(yield func(time.Time) bool) {
		for ; start.Before(end); start = start.Add(time.Hour * 24) {
			if this.Is(start) {
				if !yield(start) {
					return
				}
			}
		}
	}
}

// WorkdayModel 工作日
type WorkdayModel struct {
	ID   int64  `json:"id"`   //主键
	Unix int64  `json:"unix"` //时间戳
	Date string `json:"date"` //日期
}

func (this *WorkdayModel) TableName() string {
	return "workday"
}

func IntegerDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
