// Package workday 根据上证指数日线维护中国市场历史交易日缓存。
package workday

import (
	"errors"
	"fmt"
	"iter"
	"path/filepath"
	"reflect"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/base/maps"
	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/internal/lifecycle"
	"github.com/injoyai/tdx/v2/internal/metadata"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

// DefaultSpec 是缓存每日刷新的默认定时规则。
const DefaultSpec = "0 3 9 * * *"

type (
	Option   func(w *Workday)
	DialFunc func(c tdx.Client) (*Workday, error)
)

func WithSpec(spec string) Option {
	return func(w *Workday) {
		w.spec = spec
	}
}

func WithRetry(retry int) Option {
	return func(w *Workday) {
		w.retry = retry
	}
}

func WithDB(db *xorms.Engine) Option {
	return func(w *Workday) {
		w.db = db
	}
}

func WithDialDB(dial xorms.DialFunc) Option {
	return func(w *Workday) {
		w.dialDB = dial
	}
}

func WithClient(c tdx.Client) Option {
	return func(w *Workday) {
		w.c = clientOrNil(c)
	}
}

func WithDialClient(dial tdx.DialClientFunc) Option {
	return func(w *Workday) {
		w.dialClient = dial
	}
}

func WithOptions(op ...Option) Option {
	return func(w *Workday) {
		for _, v := range op {
			v(w)
		}
	}
}

// New 创建并立即刷新历史交易日缓存，再启动定时更新。
func New(op ...Option) (result *Workday, err error) {

	w := &Workday{
		spec:       DefaultSpec,
		retry:      tdx.DefaultRetry,
		dialDB:     nil,
		dialClient: nil,

		c:     nil,
		db:    nil,
		cache: maps.NewBit(),
	}

	for _, v := range op {
		v(w)
	}

	ownedClient, ownedDB := w.c == nil, w.db == nil
	w.cleanup = func() error {
		var err error
		if ownedClient && clientOrNil(w.c) != nil {
			err = errors.Join(err, w.c.Close())
		}
		if ownedDB && w.db != nil && w.db.Engine != nil {
			err = errors.Join(err, w.db.Close())
		}
		return err
	}
	ready := false
	defer func() {
		if !ready {
			err = errors.Join(err, w.Close())
		}
	}()

	if w.db == nil {
		if w.dialDB == nil {
			w.dialDB = func() (*xorms.Engine, error) {
				return xorms.NewSQLite(filepath.Join(metadata.DefaultDatabaseDir, "workday.db"))
			}
		}
		w.db, err = w.dialDB()
		if err != nil {
			return nil, err
		}
	}
	if w.db == nil || w.db.Engine == nil {
		return nil, errors.New("数据库工厂返回空引擎")
	}
	if err := w.db.Sync2(new(WorkdayModel)); err != nil {
		return nil, err
	}

	if w.c == nil {
		if w.dialClient == nil {
			w.dialClient = func() (tdx.Client, error) { return tdx.DialDefault() }
		}
		w.c, err = w.dialClient()
		if err != nil {
			return nil, err
		}
		w.c = clientOrNil(w.c)
		if w.c == nil {
			return nil, errors.New("client is nil")
		}
	}

	//设置定时器,每天早上9点更新数据,8点多获取不到今天的数据
	w.stop, err = update.StartTimer(w.spec, w.retry, w)
	if err != nil {
		return nil, err
	}
	ready = true
	return w, nil
}

var _ tdx.TradingCalendar = (*Workday)(nil)

// Workday 维护上证指数日线可见的历史交易日；使用完毕后调用 Close。
type Workday struct {
	state      lifecycle.Updates
	stop       func()
	cleanup    func() error
	spec       string
	retry      int
	dialDB     xorms.DialFunc
	dialClient tdx.DialClientFunc

	c     tdx.Client
	db    *xorms.Engine
	cache maps.Bit
}

// Update 更新
func (w *Workday) Update() error {
	return w.state.Run(w.refresh)
}

func (w *Workday) refresh() error {

	if w.c == nil {
		return errors.New("client is nil")
	}

	//获取沪市指数的日K线,用作历史是否节假日的判断依据
	//判断日K线是否拉取过

	//获取全部工作日
	all := []*WorkdayModel(nil)
	if err := w.db.Asc("Unix").Find(&all); err != nil {
		return err
	}
	var lastWorkday = &WorkdayModel{}
	if len(all) > 0 {
		lastWorkday = all[len(all)-1]
	}
	for _, v := range all {
		w.cache.Set(uint64(v.Unix), true)
	}

	now := time.Now().In(protocol.Location())
	if lastWorkday.Unix < tdx.IntegerDay(now).Unix() {
		resp, err := w.c.GetKlineDayAll("sh000001")
		if err != nil {
			logs.Err(err)
			return err
		}

		inserts := []any(nil)
		for _, v := range resp.List {
			day, err := time.ParseInLocation(time.DateOnly, v.Time, protocol.Location())
			if err != nil || day.Format(time.DateOnly) != v.Time {
				return fmt.Errorf("workday: 无效源日期 %q", v.Time)
			}
			// 维持已有日历数据库及 Is 使用的固定东八区 15:00 键。
			if unix := day.Add(15 * time.Hour).Unix(); unix > lastWorkday.Unix {
				inserts = append(inserts, &WorkdayModel{Unix: unix, Date: day.Format("20060102")})
			}
		}

		if len(inserts) == 0 {
			return nil
		}

		_, err = w.db.Insert(inserts)
		if err != nil {
			return err
		}
		for _, row := range inserts {
			w.cache.Set(uint64(row.(*WorkdayModel).Unix), true)
		}

	}

	return nil
}

// Is 是否是工作日
func (w *Workday) Is(t time.Time) bool {
	t = t.In(protocol.Location())
	return w.cache.Get(uint64(tdx.IntegerDay(t).Add(time.Hour * 15).Unix()))
}

// TodayIs 今天是否是工作日
func (w *Workday) TodayIs() bool {
	return w.Is(time.Now())
}

// RangeYear 遍历一年的所有工作日
func (w *Workday) RangeYear(year int, f func(t time.Time) bool) {
	w.Range(
		time.Date(year, 1, 1, 0, 0, 0, 0, protocol.Location()),
		time.Date(year, 12, 31, 0, 0, 0, 1, protocol.Location()),
		f,
	)
}

// Range 遍历指定范围的工作日,推荐start带上时间15:00,这样当天小于15点不会触发
func (w *Workday) Range(start, end time.Time, f func(t time.Time) bool) {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	for ; start.Before(end); start = start.Add(time.Hour * 24) {
		if w.Is(start) {
			if !f(start) {
				return
			}
		}
	}
}

func (w *Workday) IterYear(year int, desc ...bool) iter.Seq[time.Time] {
	return w.Iter(
		time.Date(year, 1, 1, 0, 0, 0, 0, protocol.Location()),
		time.Date(year, 12, 31, 0, 0, 0, 1, protocol.Location()),
		desc...,
	)
}

// Iter 遍历 [start,end) 内从 start 起每隔 24 小时的工作日；倒序仅改变顺序。
func (w *Workday) Iter(start, end time.Time, desc ...bool) iter.Seq[time.Time] {
	start = conv.Select(start.Before(protocol.ExchangeEstablish), protocol.ExchangeEstablish, start)
	if len(desc) > 0 && desc[0] {
		//倒序遍历
		return func(yield func(time.Time) bool) {
			if !start.Before(end) {
				return
			}
			last := start.Add((end.Sub(start) - 1) / (24 * time.Hour) * (24 * time.Hour))
			for at := last; !at.Before(start); at = at.Add(-24 * time.Hour) {
				if w.Is(at) {
					if !yield(at) {
						return
					}
				}
			}
		}
	}
	//正序遍历
	return func(yield func(time.Time) bool) {
		for at := start; at.Before(end); at = at.Add(time.Hour * 24) {
			if w.Is(at) {
				if !yield(at) {
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

func (*WorkdayModel) TableName() string {
	return "workday"
}

// Close 停止更新并释放自建资源；外部注入的客户端和数据库由调用方关闭。
func (w *Workday) Close() error { return w.state.Close(w.stop, w.cleanup) }

func clientOrNil(c tdx.Client) tdx.Client {
	if c != nil && reflect.ValueOf(c).Kind() == reflect.Pointer && reflect.ValueOf(c).IsNil() {
		return nil
	}
	return c
}
