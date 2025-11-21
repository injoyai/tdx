package xorms

import (
	"github.com/injoyai/conv"
	"github.com/injoyai/conv/cfg"
	"time"
	"xorm.io/core"
	"xorm.io/xorm"
	"xorm.io/xorm/names"
)

type Option func(*xorm.Engine)

func WithCfg(path ...string) Option {
	return WithDMap(cfg.Default.GetDMap(conv.Default[string]("database", path...)))
}

func WithDMap(m *conv.Map) Option {
	return func(e *xorm.Engine) {
		if v := m.GetVar("fieldSync"); !v.IsNil() {
			WithSyncField(v.Bool())(e)
		}
		if v := m.GetVar("tablePrefix"); !v.IsNil() {
			WithTablePrefix(v.String())(e)
		}
		if v := m.GetVar("connMaxLifetime"); !v.IsNil() {
			WithConnMaxLifetime(v.Duration())(e)
		}
		if v := m.GetVar("maxIdleConns"); !v.IsNil() {
			WithMaxIdleConns(v.Int())(e)
		}
		if v := m.GetVar("maxOpenConns"); !v.IsNil() {
			WithMaxOpenConns(v.Int())(e)
		}
	}
}

func WithTablePrefix(prefix string) Option {
	return func(e *xorm.Engine) {
		e.SetTableMapper(core.NewPrefixMapper(core.SameMapper{}, prefix))
	}
}

func WithSyncField(b bool) Option {
	return func(e *xorm.Engine) {
		if b {
			e.SetMapper(core.SameMapper{})
		} else {
			e.SetMapper(names.NewCacheMapper(new(names.SnakeMapper)))
		}
	}
}

func WithConnMaxLifetime(d time.Duration) Option {
	return func(e *xorm.Engine) {
		e.DB().SetConnMaxLifetime(d)
	}
}

func WithMaxIdleConns(n int) Option {
	return func(e *xorm.Engine) {
		e.DB().SetMaxIdleConns(n)
	}
}

func WithMaxOpenConns(n int) Option {
	return func(e *xorm.Engine) {
		e.DB().SetMaxOpenConns(n)
	}
}
