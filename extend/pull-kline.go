package extend

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/bar"
	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
	"github.com/robfig/cron/v3"
	"xorm.io/xorm"
)

const (
	Day    = "day"
	Minute = "minute"

	DirMinute = "min-kline"
	DirDay    = "day-kline"
)

type PullKlineConfig struct {
	Codes      []string  //操作代码
	Types      []string  //更新类型
	Dir        string    //数据位置
	Goroutines int       //协程数量
	StartAt    time.Time //数据开始时间
}

func NewPullKline(cfg PullKlineConfig) (*PullKline, error) {
	if cfg.Goroutines <= 0 {
		cfg.Goroutines = 1
	}
	if len(cfg.Dir) == 0 {
		cfg.Dir = filepath.Join(tdx.DefaultDatabaseDir)
	}

	db, err := xorms.NewSqlite(filepath.Join(cfg.Dir, "update.db"))
	if err != nil {
		return nil, err
	}

	updated, err := tdx.NewUpdated(db, 15, 1)
	if err != nil {
		return nil, err
	}

	return &PullKline{
		Config:  cfg,
		Updated: updated,
		Types:   cfg.Types,
	}, nil
}

type PullKline struct {
	Config  PullKlineConfig
	Updated *tdx.Updated
	Types   []string
}

func (this *PullKline) Run(m *tdx.Manage, spec string) error {
	this.Update(m, true)
	cr := cron.New(cron.WithSeconds())
	_, err := cr.AddFunc(spec, func() { this.Update(m) })
	return err
}

func (this *PullKline) Update(m *tdx.Manage, must ...bool) error {
	if len(must) == 0 || !must[0] {
		if !m.Workday.TodayIs() {
			return nil
		}
	}

	updated, err := this.Updated.Updated("pull")
	if err != nil {
		return err
	}
	if updated {
		return nil
	}

	codes := this.Config.Codes
	if len(codes) == 0 {
		codes = m.Codes.GetStockCodes()
		codes = append(codes, m.Codes.GetETFCodes()...)
		codes = append(codes, m.Codes.GetIndexCodes()...)
	}
	for _, v := range this.Types {
		switch v {
		case Day:
			err := this.updateDayKline(m, codes)
			if err != nil {
				return err
			}
		case Minute:
			err := this.updateMinKline(m, codes)
			if err != nil {
				return err
			}
		}
	}
	err = this.Updated.Update("pull")
	return err
}

func (this *PullKline) Name() string {
	return "拉取k线数据"
}

// DayKline 获取任意一天的数据,默认最新一天,即n=-1,同python支持负数
func (this *PullKline) DayKline(code string, n ...int) (*Kline, error) {
	ks, err := this.DayKlines(code, time.Time{}, time.Now())
	if err != nil {
		return nil, err
	}

	_n := conv.Default(-1, n...)
	if _n >= 0 {
		//数量不满足
		if len(ks) <= _n {
			return nil, err
		}
		return ks[_n], nil
	}

	if len(ks) < -_n {
		return nil, err
	}

	return ks[len(ks)+_n], nil
}

func (this *PullKline) DayKlinesAll(code string) (Klines, error) {
	filename := filepath.Join(this.Config.Dir, DirDay, code+".db")

	db, err := xorms.NewSqlite(filename)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	data := Klines{}
	err = db.Asc("Unix").Find(&data)
	return data, err
}

func (this *PullKline) DayKlines(code string, start, end time.Time) (Klines, error) {
	filename := filepath.Join(this.Config.Dir, DirDay, code+".db")

	db, err := xorms.NewSqlite(filename)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	data := Klines{}
	err = db.Where("Unix >= ? and Unix <= ?", start.Unix(), end.Unix()).Asc("Unix").Find(&data)
	return data, err
}

func (this *PullKline) MinKlines(code string, start, end time.Time) (protocol.Klines, error) {
	years := []int(nil)
	for i := start.Year(); i <= end.Year(); i++ {
		years = append(years, i)
	}
	ks := protocol.Klines{}
	mu := sync.Mutex{}
	wg := sync.WaitGroup{}
	for _, year := range years {
		wg.Add(1)
		go func(code string, year int) {
			defer wg.Done()
			filename := filepath.Join(this.Config.Dir, DirMinute, code, code+"-"+strconv.Itoa(year)+".db")
			if !exists(filename) {
				return
			}
			db, err := xorms.NewSqlite(filename)
			if err != nil {
				logs.Err(err)
				return
			}
			defer db.Close()
			ls := protocol.Klines{}
			err = db.Find(&ls)
			if err != nil {
				logs.Err(err)
				return
			}
			res := protocol.Klines{}
			for _, l := range ls {
				if l.Time.Year() != year {
					continue
				}
				res = append(res, l)
			}
			mu.Lock()
			defer mu.Unlock()
			ks = append(ks, res...)
		}(code, year)
	}
	wg.Wait()
	ks.Sort()
	return ks, nil
}

func (this *PullKline) updateDayKline(m *tdx.Manage, codes []string) error {

	_ = os.MkdirAll(this.Config.Dir, os.ModePerm)

	b := bar.NewCoroutine(len(codes), this.Config.Goroutines, bar.WithPrefix("[xx000000]"))
	defer b.Close()

	for i := range codes {

		code := codes[i]

		b.GoRetry(func() (err error) {

			b.SetPrefix(fmt.Sprintf("[%s]", code))
			b.Flush()

			defer func() {
				if err != nil {
					b.Logf("[错误] [%s] %s\n", code, err)
					b.Flush()
				}
			}()

			//连接数据库
			db, err := xorms.NewSqlite(filepath.Join(this.Config.Dir, DirDay, code+".db"))
			if err != nil {
				return err
			}
			defer db.Close()

			if err = db.Sync2(new(Kline)); err != nil {
				return err
			}

			//2. 获取最后一条数据
			last := new(Kline)
			if _, err = db.Desc("Unix").Get(last); err != nil {
				return err
			}

			//3. 从服务器获取数据
			var resp *protocol.KlineResp
			err = m.Do(func(c *tdx.Client) error {
				resp, err = c.GetKlineDayUntil(code, func(k *protocol.Kline) bool {
					return k.Time.Before(last.Time) || k.Time.Before(this.Config.StartAt)
				})
				return err
			})
			if err != nil {
				return err
			}

			//4. 插入数据库
			err = db.SessionFunc(func(session *xorm.Session) error {
				if _, er := session.Where("Unix >= ?", last.Time.Unix()).Delete(new(Kline)); er != nil {
					return er
				}
				for _, v := range resp.List {
					if v.Time.Before(last.Time) {
						continue
					}
					k := &Kline{
						Unix:       v.Time.Unix(),
						Kline:      v,
						Turnover:   0,
						FloatStock: 0,
						TotalStock: 0,
					}
					if eq := m.Gbbq.GetEquity(code, v.Time); eq != nil {
						k.Turnover = eq.Turnover(v.Volume * 100)
						k.FloatStock = eq.Float
						k.TotalStock = eq.Total
					}
					if _, er := session.Insert(k); er != nil {
						return er
					}
				}
				return nil
			})

			return

		}, tdx.DefaultRetry)

	}

	b.Wait()
	return nil
}

func (this *PullKline) updateMinKline(m *tdx.Manage, codes []string) error {

	_ = os.MkdirAll(this.Config.Dir, os.ModePerm)

	b := bar.NewCoroutine(len(codes), this.Config.Goroutines, bar.WithPrefix("[xx000000]"))
	defer b.Close()

	year := time.Now().Year()

	for i := range codes {

		code := codes[i]

		b.GoRetry(func() (err error) {

			b.SetPrefix(fmt.Sprintf("[%s]", code))
			b.Flush()

			defer func() {
				if err != nil {
					b.Logf("[错误] [%s] %s\n", code, err)
					b.Flush()
				}
			}()

			ks := protocol.Klines{}
			//判断数据库文件是否存在,如果今年的数据库文件不存在,则按年向前填充
			filename := filepath.Join(this.Config.Dir, DirMinute, code, code+"-"+conv.String(year)+".db")
			if !exists(filename) {
				//尝试更新去年的数据
				ks, err = this.updateMinuteKlineYear(m, code, year-1, ks)
				if err != nil {
					return err
				}
			}
			//更新今年的数据
			_, err = this.updateMinuteKlineYear(m, code, year, ks)

			return

		}, tdx.DefaultRetry)

	}

	b.Wait()
	return nil
}

func (this *PullKline) updateMinuteKlineYear(m *tdx.Manage, code string, year int, ks protocol.Klines) (protocol.Klines, error) {
	//去年的数据库文件
	filename := filepath.Join(this.Config.Dir, DirMinute, code, code+"-"+conv.String(year)+".db")

	db, err := xorms.NewSqlite(filename)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err = db.Sync2(new(protocol.Kline)); err != nil {
		return nil, err
	}

	//获取最新一条数据
	last := new(protocol.Kline)
	_, err = db.Desc("Time").Get(last)
	if err != nil {
		return nil, err
	}

	if len(ks) == 0 {
		err = m.Do(func(c *tdx.Client) error {
			resp, err := c.GetKlineMinute241Until(code, func(k *protocol.Kline) bool {
				return k.Time.Before(last.Time)
			})
			if err != nil {
				return err
			}
			ks = resp.List
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	err = db.SessionFunc(func(session *xorm.Session) error {
		if _, err := session.Where("Time=?", last.Time.UTC().Format(time.DateTime)).Delete(new(protocol.Kline)); err != nil {
			return err
		}
		for _, v := range ks {
			if v.Time.Before(last.Time) || v.Time.After(time.Date(year+1, 1, 1, 0, 0, 0, 0, time.Local)) {
				continue
			}
			if _, err = session.Insert(v); err != nil {
				return err
			}
		}
		return nil
	})

	return ks, err
}
