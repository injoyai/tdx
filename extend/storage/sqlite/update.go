package sqlite

import "xorm.io/xorm"

// updateRecord 保持 SQLite 历史表结构。
type updateRecord struct {
	Key  string
	Time int64
}

func (*updateRecord) TableName() string { return "update" }

func (s *PullStore) LoadUpdate(key string) (int64, bool, error) {
	var r updateRecord
	has, err := s.updatedDB.Where("`"+s.updatedDB.GetColumnMapper().Obj2Table("Key")+"`=?", key).Get(&r)
	return r.Time, has, err
}

func (s *PullStore) SaveUpdate(key string, unix int64) error {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	_, err := s.updatedDB.Transaction(func(tx *xorm.Session) (any, error) {
		mark := &updateRecord{Key: key, Time: unix}
		n, err := tx.Where("`"+s.updatedDB.GetColumnMapper().Obj2Table("Key")+"`=?", key).
			Cols(s.updatedDB.GetColumnMapper().Obj2Table("Time")).Update(mark)
		if err != nil || n > 0 {
			return nil, err
		}
		_, err = tx.Insert(mark)
		return nil, err
	})
	return err
}
