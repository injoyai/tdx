package sqlite

import (
	"fmt"
	"path/filepath"

	"github.com/injoyai/tdx/v2/extend/model"
	"xorm.io/xorm"
)

type emptyMinuteScan struct {
	Key       string `xorm:"pk"`
	From      string
	CheckedAt int64
}

func emptyScanKey(code model.Code, year int) string {
	return fmt.Sprintf("%s.%s:%d", code.Market, code.Code, year)
}

func (s *PullStore) LoadMinuteEmptyScan(code model.Code, year int) (*model.MinuteEmptyScan, error) {
	if err := validateMinuteCode(code, year); err != nil {
		return nil, err
	}
	file := filepath.Join(s.dir, "minute-scans.db")
	if !exists(file) {
		return nil, nil
	}
	db, release, err := s.engine(file, new(emptyMinuteScan))
	if err != nil {
		return nil, err
	}
	defer release()
	var scan emptyMinuteScan
	has, err := db.ID(emptyScanKey(code, year)).Get(&scan)
	if err != nil || !has {
		return nil, err
	}
	return &model.MinuteEmptyScan{From: scan.From, CheckedAt: scan.CheckedAt}, nil
}

func (s *PullStore) SaveMinuteEmptyScan(code model.Code, year int, scan model.MinuteEmptyScan) error {
	if err := validateMinuteCode(code, year); err != nil {
		return err
	}
	db, release, err := s.engine(filepath.Join(s.dir, "minute-scans.db"), new(emptyMinuteScan))
	if err != nil {
		return err
	}
	defer release()
	row := &emptyMinuteScan{Key: emptyScanKey(code, year), From: scan.From, CheckedAt: scan.CheckedAt}
	return transaction(db, func(session *xorm.Session) error {
		if _, err := session.ID(row.Key).Delete(new(emptyMinuteScan)); err != nil {
			return err
		}
		_, err := session.Insert(row)
		return err
	})
}
