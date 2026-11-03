// Package codes 提供标准市场的代码查询接口、内存代码表和持久化自动更新缓存。
package codes

import (
	"errors"
	"path/filepath"
	"reflect"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/internal/lifecycle"
	"github.com/injoyai/tdx/v2/internal/metadata"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

// DefaultSpec 是缓存每日刷新的默认定时规则。
const DefaultSpec = "0 1 9 * * *"

type (
	Option func(*Codes)

	DialFunc func(c tdx.Client) (Interface, error)
)

// WithStore 注入自定义持久化；存储由调用方关闭，不能与 WithDB/WithDialDB 同时使用。
func WithStore(store Store) Option {
	return func(s *Codes) { s.store = store }
}

func WithDB(db *xorms.Engine) Option {
	return func(c *Codes) {
		c.db = db
	}
}

func WithDialDB(dial xorms.DialFunc) Option {
	return func(c *Codes) {
		c.dialDB = dial
	}
}

func WithSpec(spec string) Option {
	return func(c *Codes) {
		c.spec = spec
	}
}

func WithRetry(retry int) Option {
	return func(c *Codes) {
		c.retry = retry
	}
}

func WithClient(c tdx.Client) Option {
	return func(cs *Codes) {
		cs.c = clientOrNil(c)
	}
}

func WithDialClient(dial tdx.DialClientFunc) Option {
	return func(c *Codes) {
		c.dialClient = dial
	}
}

func WithOptions(op ...Option) Option {
	return func(c *Codes) {
		for _, v := range op {
			if v != nil {
				v(c)
			}
		}
	}
}

// New 创建并立即刷新代码缓存，再启动定时更新。自建资源由 Close 释放。
func New(op ...Option) (result *Codes, err error) {
	cs := &Codes{
		spec:      DefaultSpec,
		retry:     tdx.DefaultRetry,
		Base:      NewBase(),
		updateKey: "codes",
	}

	WithOptions(op...)(cs)

	if cs.store != nil && (cs.db != nil || cs.dialDB != nil) {
		return nil, errors.New("codes: WithStore 不能与 WithDB/WithDialDB 同时使用")
	}
	ownedClient, ownedDB := cs.c == nil, cs.db == nil
	cs.cleanup = func() error {
		var err error
		if ownedClient && clientOrNil(cs.c) != nil {
			err = errors.Join(err, cs.c.Close())
		}
		if ownedDB && cs.db != nil && cs.db.Engine != nil {
			err = errors.Join(err, cs.db.Close())
		}
		return err
	}
	ready := false
	defer func() {
		if !ready {
			err = errors.Join(err, cs.Close())
		}
	}()

	// 初始化连接
	if cs.c == nil {
		if cs.dialClient == nil {
			cs.dialClient = func() (tdx.Client, error) { return tdx.DialDefault() }
		}
		cs.c, err = cs.dialClient()
		if err != nil {
			return nil, err
		}
		cs.c = clientOrNil(cs.c)
		if cs.c == nil {
			return nil, errors.New("client is nil")
		}
	}

	// 初始化数据库
	if cs.store == nil {
		if cs.db == nil {
			if cs.dialDB == nil {
				cs.dialDB = func() (*xorms.Engine, error) {
					return xorms.NewSQLite(filepath.Join(metadata.DefaultDatabaseDir, "codes.db"))
				}
			}
			cs.db, err = cs.dialDB()
			if err != nil {
				return nil, err
			}
		}
		if cs.db == nil || cs.db.Engine == nil {
			return nil, errors.New("数据库工厂返回空引擎")
		}
		cs.store, err = NewSQLStore(cs.db)
		if err != nil {
			return nil, err
		}
	}
	cs.updated, err = update.NewWithStore(cs.store, 9, 0)
	if err != nil {
		return nil, err
	}

	// 立即/定时更新
	cs.stop, err = update.StartTimer(cs.spec, cs.retry, cs)
	if err != nil {
		return nil, err
	}
	ready = true
	return cs, nil
}

var _ Interface = &Codes{}

// Codes 在内存中提供代码表查询，并通过 Store 持久化和定时任务刷新。
type Codes struct {
	state   lifecycle.Updates
	stop    func()
	cleanup func() error
	spec    string //定时规则
	retry   int    //重试次数

	dialDB     xorms.DialFunc
	dialClient tdx.DialClientFunc

	c         tdx.Client
	store     Store
	db        *xorms.Engine
	updateKey string
	updated   *update.Tracker

	*Base
}

func (c *Codes) Update() error {
	return c.state.Run(func() error {
		codes, err := c.update()
		if err != nil {
			return err
		}
		c.Base.Update(codes)
		return nil
	})
}

// update 在完整拉取成功后提交数据库，再返回持久化快照。
func (c *Codes) update() ([]*CodeModel, error) {

	if c.c == nil {
		return nil, errors.New("client is nil")
	}

	//2. 查询数据库所有股票
	list, err := c.store.Load()
	if err != nil {
		return nil, err
	}

	//如果更新过,则不更新
	updated, err := c.updated.IsUpdated(c.updateKey)
	if err != nil {
		return nil, err
	}
	if updated {
		return list, nil
	}

	//3. 从服务器获取所有股票代码
	fresh := []*CodeModel(nil)
	for _, exchange := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ} {
		resp, err := c.c.GetCodeAll(exchange)
		if err != nil {
			return nil, err
		}
		for _, item := range resp.List {
			v := item.Standard
			if v == nil {
				return nil, errors.New("代码管理需要标准市场原始数据")
			}
			code := &CodeModel{
				Name:      v.Name,
				Code:      v.Code,
				Exchange:  exchange.String(),
				Multiple:  v.Multiple,
				Decimal:   v.Decimal,
				LastPrice: v.LastPrice,
			}
			fresh = append(fresh, code)
		}
	}

	list, err = c.store.Save(fresh)
	if err != nil {
		return nil, err
	}

	//更新时间
	err = c.updated.Mark(c.updateKey)
	return list, err
}

// Close 停止更新并释放自建资源；外部注入的客户端和数据库仍由调用方关闭。
func (c *Codes) Close() error { return c.state.Close(c.stop, c.cleanup) }

func clientOrNil(c tdx.Client) tdx.Client {
	if c != nil && reflect.ValueOf(c).Kind() == reflect.Pointer && reflect.ValueOf(c).IsNil() {
		return nil
	}
	return c
}
