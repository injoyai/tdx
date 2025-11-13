package xorms

import (
	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	"os"
	"path/filepath"
	"time"
	"xorm.io/core"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

func NewMysql(dsn string, options ...Option) (*Engine, error) {
	return New("mysql", dsn, options...)
}

func NewSqlite(filename string, options ...Option) (*Engine, error) {
	dir, _ := filepath.Split(filename)
	_ = os.MkdirAll(dir, 0777)
	//sqlite是文件数据库,只能打开一次(即一个连接)
	options = append(options, WithMaxOpenConns(1))
	return New("sqlite", filename, options...)
}

/*
New 需要手动引用驱动
mysql _ "github.com/go-sql-driver/mysql"
sqlite _ "github.com/glebarez/go-sqlite"
sqlserver _ "github.com/denisenkom/go-mssqldb"
*/
func New(Type, dsn string, options ...Option) (*Engine, error) {
	db, err := xorm.NewEngine(Type, dsn)
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

func (this *Engine) TableName(v any) string {
	return this.Engine.TableName(v)
}

func (this *Engine) Tables() []*schemas.Table {
	list, _ := this.DBMetas()
	return list
}

// SetTablePrefix 前缀
func (this *Engine) SetTablePrefix(s string) *Engine {
	this.SetTableMapper(core.NewPrefixMapper(core.SameMapper{}, s))
	return this
}

// SetSyncField 字段同步
func (this *Engine) SetSyncField() *Engine {
	this.SetMapper(core.SameMapper{})
	return this
}

// SetConnMaxLifetime 设置连接超时时间(超时会断开连接)
func (this *Engine) SetConnMaxLifetime(d time.Duration) *Engine {
	this.DB().SetConnMaxLifetime(d)
	return this
}

// SetMaxIdleConns 设置空闲数(一直连接不断开)
func (this *Engine) SetMaxIdleConns(n int) *Engine {
	this.DB().SetMaxIdleConns(n)
	return this
}

// SetMaxOpenConns 设置连接数(超出最大数量会等待)
func (this *Engine) SetMaxOpenConns(n int) *Engine {
	this.DB().SetMaxOpenConns(n)
	return this
}

// NewSession 新建自动关闭事务
func (this *Engine) NewSession() *Session {
	return newSession(this.Engine.Where(""))
}

func (this *Engine) SessionFunc(fn func(session *xorm.Session) error) error {
	return NewSessionFunc(this.Engine, fn)
}

func (this *Engine) Like(param, arg string) *Session {
	return newSession(this.Engine.Where(param+" like ?", "%"+arg+"%"))
}

func (this *Engine) Desc(colNames ...string) *Session {
	return newSession(this.Engine.Desc(colNames...))
}

func (this *Engine) Asc(colNames ...string) *Session {
	return newSession(this.Engine.Asc(colNames...))
}

func (this *Engine) Limit(limit int, start ...int) *Session {
	if limit > 0 {
		return newSession(this.Engine.Limit(limit, start...))
	}
	return newSession(this.Engine.Where(""))
}

func (this *Engine) Where(query any, args ...any) *Session {
	return newSession(this.Engine.Where(query, args...))
}
