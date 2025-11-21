package xorms

import "xorm.io/xorm"

type Session struct {
	*xorm.Session
}

func newSession(session *xorm.Session) *Session {
	return &Session{session}
}

func (this *Session) Like(param, arg string) *Session {
	this.Session.Where(param+" like ?", "%"+arg+"%")
	return this
}

func (this *Session) Desc(colNames ...string) *Session {
	this.Session.Desc(colNames...)
	return this
}

func (this *Session) Asc(colNames ...string) *Session {
	this.Session.Asc(colNames...)
	return this
}

func (this *Session) Limit(limit int, start ...int) *Session {
	if limit > 0 {
		this.Session.Limit(limit, start...)
	}
	return this
}

func (this *Session) Where(query any, args ...any) *Session {
	this.Session.Where(query, args...)
	return this
}

func (this *Session) And(query any, args ...any) *Session {
	this.Session.And(query, args...)
	return this
}

func NewSessionFunc(db *xorm.Engine, fn func(session *xorm.Session) error) error {
	session := db.NewSession()
	defer session.Close()
	if err := session.Begin(); err != nil {
		session.Rollback()
		return err
	}
	if err := fn(session); err != nil {
		session.Rollback()
		return err
	}
	if err := session.Commit(); err != nil {
		session.Rollback()
		return err
	}
	return nil
}
