package extend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/injoyai/bar"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
	"xorm.io/xorm"
)

const (
	Minute   = "minute"
	Minute5  = "5minute"
	Minute15 = "15minute"
	Minute30 = "30minute"
	Minute60 = "60minute"
	Day      = "day"
	Week     = "week"
	Month    = "month"
	Quarter  = "quarter"
	Year     = "year"

	TableMinute   = "MinuteKline"
	Table5Minute  = "Minute5Kline"
	Table15Minute = "Minute15Kline"
	Table30Minute = "Minute30Kline"
	Table60Minute = "Minute60Kline"
	TableDay      = "DayKline"
	TableWeek     = "WeekKline"
	TableMonth    = "MonthKline"
	TableQuarter  = "QuarterKline"
	TableYear     = "YearKline"
)

var (
	AllTable      = []string{Minute, Minute5, Minute15, Minute30, Minute60, Day, Week, Month, Quarter, Year}
	AllKlineType  = AllTable //向下兼容
	KlineTableMap = map[string]*KlineTable{
		Minute:   NewKlineTable(TableMinute, func(c *tdx.Client) KlineHandler { return c.GetKlineMinuteUntil }),
		Minute5:  NewKlineTable(Table5Minute, func(c *tdx.Client) KlineHandler { return c.GetKline5MinuteUntil }),
		Minute15: NewKlineTable(Table15Minute, func(c *tdx.Client) KlineHandler { return c.GetKline15MinuteUntil }),
		Minute30: NewKlineTable(Table30Minute, func(c *tdx.Client) KlineHandler { return c.GetKline30MinuteUntil }),
		Minute60: NewKlineTable(Table60Minute, func(c *tdx.Client) KlineHandler { return c.GetKline60MinuteUntil }),
		Day:      NewKlineTable(TableDay, func(c *tdx.Client) KlineHandler { return c.GetKlineDayUntil }),
		Week:     NewKlineTable(TableWeek, func(c *tdx.Client) KlineHandler { return c.GetKlineWeekUntil }),
		Month:    NewKlineTable(TableMonth, func(c *tdx.Client) KlineHandler { return c.GetKlineMonthUntil }),
		Quarter:  NewKlineTable(TableQuarter, func(c *tdx.Client) KlineHandler { return c.GetKlineQuarterUntil }),
		Year:     NewKlineTable(TableYear, func(c *tdx.Client) KlineHandler { return c.GetKlineYearUntil }),
	}
)

type PullKlineConfig struct {
	Codes      []string  //操作代码
	Tables     []string  //数据类型
	Dir        string    //数据位置
	Goroutines int       //协程数量
	StartAt    time.Time //数据开始时间
}

func NewPullKline(cfg PullKlineConfig) *PullKline {
	_tables := []*KlineTable(nil)
	for _, v := range cfg.Tables {
		_tables = append(_tables, KlineTableMap[v])
	}
	if cfg.Goroutines <= 0 {
		cfg.Goroutines = 1
	}
	if len(cfg.Dir) == 0 {
		cfg.Dir = filepath.Join(tdx.DefaultDatabaseDir, "kline")
	}
	return &PullKline{
		tables: _tables,
		Config: cfg,
	}
}

type PullKline struct {
	tables []*KlineTable
	Config PullKlineConfig
}

func (this *PullKline) Name() string {
	return "拉取k线数据"
}

func (this *PullKline) DayKlines(code string) (Klines, error) {
	return this.Klines(TableDay, code)
}

func (this *PullKline) MinKlines(code string) (Klines, error) {
	return this.Klines(TableMinute, code)
}

func (this *PullKline) Klines(table string, code string) (Klines, error) {
	//连接数据库
	db, err := xorms.NewSqlite(filepath.Join(this.Config.Dir, code+".db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	data := Klines{}
	err = db.Table(table).Asc("Unix").Find(&data)
	return data, err
}

func (this *PullKline) Update(m *tdx.Manage) error {

	_ = os.MkdirAll(this.Config.Dir, 0777)

	//1. 获取所有股票代码
	codes := this.Config.Codes
	if len(codes) == 0 {
		codes = m.Codes.GetStockCodes()
	}

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
			db, err := xorms.NewSqlite(filepath.Join(this.Config.Dir, code+".db"))
			if err != nil {
				return err
			}
			defer db.Close()

			for _, table := range this.tables {
				if table == nil {
					continue
				}

				if err = db.Sync2(table); err != nil {
					return err
				}

				//2. 获取最后一条数据
				last := new(Kline)
				if _, err = db.Table(table).Desc("Unix").Get(last); err != nil {
					return err
				}

				//3. 从服务器获取数据
				var resp *protocol.KlineResp
				err = m.Do(func(c *tdx.Client) error {
					resp, err = table.handler(c)(code, func(k *protocol.Kline) bool {
						return k.Time.Before(last.Time) || k.Time.Before(this.Config.StartAt)
					})
					return err
				})
				if err != nil {
					return err
				}

				//4. 插入数据库
				err = db.SessionFunc(func(session *xorm.Session) error {
					if _, er := session.Table(table).Where("Unix >= ?", last.Time.Unix()).Delete(); er != nil {
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
						if _, er := session.Table(table).Insert(k); er != nil {
							logs.Err(er)
							return er
						}
					}
					return nil
				})
				if err != nil {
					return err
				}

			}

			return

		}, tdx.DefaultRetry)

	}

	b.Wait()
	return nil
}

/*



 */

type KlineHandler func(code string, f func(k *protocol.Kline) bool) (*protocol.KlineResp, error)

func NewKlineTable(tableName string, handler func(c *tdx.Client) KlineHandler) *KlineTable {
	return &KlineTable{
		tableName: tableName,
		handler:   handler,
	}
}

type KlineTable struct {
	*Kline    `xorm:"extends"`                 //同步字段
	tableName string                           //同步表名
	handler   func(c *tdx.Client) KlineHandler //
}

func (this *KlineTable) TableName() string {
	return this.tableName
}
