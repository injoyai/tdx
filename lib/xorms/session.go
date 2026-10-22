package xorms

import "xorm.io/xorm"

type Session struct {
	*xorm.Session
}

func newSession(session *xorm.Session) *Session {
	return &Session{session}
}

func (s *Session) Like(param, arg string) *Session {
	s.Session.Where(param+" like ?", "%"+arg+"%")
	return s
}

func (s *Session) Desc(colNames ...string) *Session {
	s.Session.Desc(colNames...)
	return s
}

func (s *Session) Asc(colNames ...string) *Session {
	s.Session.Asc(colNames...)
	return s
}

func (s *Session) Limit(limit int, start ...int) *Session {
	if limit > 0 {
		s.Session.Limit(limit, start...)
	}
	return s
}

func (s *Session) Where(query any, args ...any) *Session {
	s.Session.Where(query, args...)
	return s
}

func (s *Session) And(query any, args ...any) *Session {
	s.Session.And(query, args...)
	return s
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
