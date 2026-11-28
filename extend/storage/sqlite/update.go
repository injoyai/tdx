package sqlite

import (
	"fmt"
	"strings"

	"xorm.io/xorm"
)

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
	// A BarStore decorator may expose only SaveUpdate. Preserve the same
	// atomic claim for that path so it cannot overwrite another store's binding.
	if unix < 0 && strings.HasPrefix(key, "pull:definition:") {
		matches, err := s.BindDefinition(key, "", unix)
		if err == nil && !matches {
			return fmt.Errorf("sqlite: definition already bound for %q", key)
		}
		return err
	}
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

// BindDefinition atomically claims a market in updated.db, across connections
// and processes. A legacy binding can be upgraded when only one legacy
// definition marker exists and it matches both the market and the requested definition.
func (s *PullStore) BindDefinition(key, legacyKey string, token int64) (bool, error) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()
	keyColumn := "`" + s.updatedDB.GetColumnMapper().Obj2Table("Key") + "`"
	timeColumn := "`" + s.updatedDB.GetColumnMapper().Obj2Table("Time") + "`"
	table := "`update`"
	result, err := s.updatedDB.Transaction(func(tx *xorm.Session) (any, error) {
		// Start the transaction with a write so competing stores cannot both see an unclaimed market.
		query := "INSERT INTO " + table + " (" + keyColumn + "," + timeColumn + ")" +
			" SELECT ?,? WHERE NOT EXISTS (SELECT 1 FROM " + table + " WHERE " + keyColumn + "=?)"
		if _, err := tx.Exec(query, key, token, key); err != nil {
			return false, err
		}
		var rows []updateRecord
		if err := tx.Where(keyColumn+"=?", key).Find(&rows); err != nil {
			return false, err
		}
		if len(rows) != 1 {
			return false, nil
		}
		current := rows[0].Time
		if current < 0 {
			return current == token, nil
		}

		prefix := key + ":"
		var candidates []updateRecord
		// Use a binary key range so names containing SQL wildcard characters remain literal.
		if err := tx.Where(keyColumn+">=? AND "+keyColumn+"<? AND length("+keyColumn+")=length(?)+64", prefix, key+";", prefix).
			Limit(2).Find(&candidates); err != nil {
			return false, err
		}
		if len(candidates) != 1 || candidates[0].Key != legacyKey || candidates[0].Time != current {
			return false, nil
		}
		_, err := tx.Where(keyColumn+"=?", key).Cols(s.updatedDB.GetColumnMapper().Obj2Table("Time")).
			Update(&updateRecord{Time: token})
		return err == nil, err
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}
