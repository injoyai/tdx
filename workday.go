package tdx

import (
	"errors"
	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/base/maps"
	"github.com/injoyai/logs"
	"github.com/robfig/cron/v3"
	"os"
	"path/filepath"
	"time"
	"xorm.io/core"
	"xorm.io/xorm"
)

func NewWorkday(c *Client, filename string) (*Workday, error) {

	//如果文件夹不存在就创建
	dir, _ := filepath.Split(filename)
	_ = os.MkdirAll(dir, 0777)

	//连接数据库
	db, err := xorm.NewEngine("sqlite", filename)
	if err != nil {
		return nil, err
	}
	db.SetMapper(core.SameMapper{})
	db.DB().SetMaxOpenConns(1)
	if err := db.Sync2(new(WorkdayModel)); err != nil {
		return nil, err
	}

	w := &Workday{
		Client: c,
		db:     db,
		cache:  maps.NewBit(),
	}

	//设置定时器,每天早上9点更新数据,8点多获取不到今天的数据
	task := cron.New(cron.WithSeconds())
	task.AddFunc("0 0 9 * * *", func() {
		for i := 0; i < 3; i++ {
			if err := w.Update(); err == nil {
				return
			}
			logs.Err(err)
			<-time.After(time.Minute * 5)
		}
	})
	task.Start()

	return w, w.Update()
}

type Workday struct {
	*Client
	db    *xorm.Engine
	cache maps.Bit
}

// Update 更新
func (this *Workday) Update() error {

	if this.Client == nil {
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
	if lastWorkday == nil || lastWorkday.Unix < IntegerDay(now).Unix() {
		resp, err := this.Client.GetIndexDayAll("sh000001")
		if err != nil {
			logs.Err(err)
			return err
		}

		return NewSessionFunc(this.db, func(session *xorm.Session) error {
			for _, v := range resp.List {
				if unix := v.Time.Unix(); unix > lastWorkday.Unix {
					_, err = session.Insert(&WorkdayModel{Unix: unix, Date: v.Time.Format("20060102"), Is: true})
					if err != nil {
						return err
					}
					this.cache.Set(uint64(unix), true)
				}
			}
			return nil
		})

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

// RangeDesc 倒序遍历工作日,从今天-1990年12月19日(上海交易所成立时间)
func (this *Workday) RangeDesc(f func(t time.Time) bool) {
	t := IntegerDay(time.Now())
	for ; t.After(time.Date(1990, 12, 18, 0, 0, 0, 0, time.Local)); t = t.Add(-time.Hour * 24) {
		if this.Is(t) {
			if !f(t) {
				return
			}
		}
	}
}

// WorkdayModel 工作日
type WorkdayModel struct {
	ID   int64  `json:"id"`   //主键
	Unix int64  `json:"unix"` //时间戳
	Date string `json:"date"` //日期
	Is   bool   `json:"is"`   //是否是工作日
}

func (this *WorkdayModel) TableName() string {
	return "workday"
}

func IntegerDay(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}
