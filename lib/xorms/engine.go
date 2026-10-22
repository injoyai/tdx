package xorms

import (
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"xorm.io/core"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// DialFunc 创建数据库引擎。
type DialFunc func() (*Engine, error)

func NewSQLite(filename string, options ...Option) (*Engine, error) {
	dir, _ := filepath.Split(filename)
	_ = os.MkdirAll(dir, 0777)
	//sqlite是文件数据库,只能打开一次(即一个连接)
	options = append(options, WithMaxOpenConns(1))
	return New("sqlite", filename, options...)
}

/*
New 需要手动引用驱动
sqlite _ "github.com/glebarez/go-sqlite"
sqlserver _ "github.com/denisenkom/go-mssqldb"
*/
func New(driver, dsn string, options ...Option) (*Engine, error) {
	db, err := xorm.NewEngine(driver, dsn)
	if err != nil {
		return nil, err
	}
	//默认同步字段
	WithSyncField(true)(db)
	for _, v := range options {
		v(db)
	}
	return &Engine{Engine: db}, nil
}

type Engine struct {
	*xorm.Engine
}

func (e *Engine) TableName(v any) string {
	return e.Engine.TableName(v)
}

func (e *Engine) Tables() []*schemas.Table {
	list, _ := e.DBMetas()
	return list
}

// SetTablePrefix 前缀
func (e *Engine) SetTablePrefix(s string) *Engine {
	e.SetTableMapper(core.NewPrefixMapper(core.SameMapper{}, s))
	return e
}

// SetSyncField 字段同步
func (e *Engine) SetSyncField() *Engine {
	e.SetMapper(core.SameMapper{})
	return e
}

// SetConnMaxLifetime 设置连接超时时间(超时会断开连接)
func (e *Engine) SetConnMaxLifetime(d time.Duration) *Engine {
	e.DB().SetConnMaxLifetime(d)
	return e
}

// SetMaxIdleConns 设置空闲数(一直连接不断开)
func (e *Engine) SetMaxIdleConns(n int) *Engine {
	e.DB().SetMaxIdleConns(n)
	return e
}

// SetMaxOpenConns 设置连接数(超出最大数量会等待)
func (e *Engine) SetMaxOpenConns(n int) *Engine {
	e.DB().SetMaxOpenConns(n)
	return e
}

// NewSession 新建自动关闭事务
func (e *Engine) NewSession() *Session {
	return newSession(e.Engine.Where(""))
}

func (e *Engine) SessionFunc(fn func(session *xorm.Session) error) error {
	return NewSessionFunc(e.Engine, fn)
}

func (e *Engine) Like(param, arg string) *Session {
	return newSession(e.Engine.Where(param+" like ?", "%"+arg+"%"))
}

func (e *Engine) Desc(colNames ...string) *Session {
	return newSession(e.Engine.Desc(colNames...))
}

func (e *Engine) Asc(colNames ...string) *Session {
	return newSession(e.Engine.Asc(colNames...))
}

func (e *Engine) Limit(limit int, start ...int) *Session {
	if limit > 0 {
		return newSession(e.Engine.Limit(limit, start...))
	}
	return newSession(e.Engine.Where(""))
}

func (e *Engine) Where(query any, args ...any) *Session {
	return newSession(e.Engine.Where(query, args...))
}
