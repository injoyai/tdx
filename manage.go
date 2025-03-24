package tdx

import (
	"github.com/injoyai/ios/client"
	"github.com/robfig/cron/v3"
	"path/filepath"
	"time"
)

const (
	DefaultDatabaseDir = "./data/database"
)

func NewManage(cfg *ManageConfig, op ...client.Option) (*Manage, error) {
	//初始化配置
	if cfg == nil {
		cfg = &ManageConfig{}
	}
	if cfg.CodesDir == "" {
		cfg.CodesDir = DefaultDatabaseDir
	}
	if cfg.WorkdayDir == "" {
		cfg.WorkdayDir = DefaultDatabaseDir
	}
	if cfg.Dial == nil {
		cfg.Dial = DialDefault
	}

	//代码
	codesClient, err := cfg.Dial(op...)
	if err != nil {
		return nil, err
	}
	codesClient.Wait.SetTimeout(time.Second * 5)
	codes, err := NewCodes(codesClient, filepath.Join(cfg.CodesDir, "codes.db"))
	if err != nil {
		return nil, err
	}

	//连接池
	p, err := NewPool(func() (*Client, error) {
		return cfg.Dial(op...)
	}, cfg.Number)
	if err != nil {
		return nil, err
	}

	//工作日
	workdayClient, err := cfg.Dial(op...)
	if err != nil {
		return nil, err
	}
	workdayClient.Wait.SetTimeout(time.Second * 5)
	workday, err := NewWorkday(workdayClient, filepath.Join(cfg.WorkdayDir, "workday.db"))
	if err != nil {
		return nil, err
	}

	return &Manage{
		Pool:    p,
		Config:  cfg,
		Codes:   codes,
		Workday: workday,
		Cron:    cron.New(cron.WithSeconds()),
	}, nil
}

type Manage struct {
	*Pool
	Config  *ManageConfig
	Codes   *Codes
	Workday *Workday
	Cron    *cron.Cron
}

// AddWorkdayTask 添加工作日任务
func (this *Manage) AddWorkdayTask(spec string, f func(m *Manage)) {
	this.Cron.AddFunc(spec, func() {
		if this.Workday.TodayIs() {
			f(this)
		}
	})
}

type ManageConfig struct {
	Number     int                                                //客户端数量
	CodesDir   string                                             //代码数据库位置
	WorkdayDir string                                             //工作日数据库位置
	Dial       func(op ...client.Option) (cli *Client, err error) //默认连接方式
}
