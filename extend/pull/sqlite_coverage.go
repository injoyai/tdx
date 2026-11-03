package pull

import "xorm.io/xorm"

// QueryMinuteCoverage 查询分钟覆盖记录；缺少记录时返回 nil。
func (s *SQLiteStore) QueryMinuteCoverage(code Code, year int) (*MinuteCoverage, error) {
	if !s.MinuteExists(code, year) {
		return nil, nil
	}
	db, release, err := s.openMinute(code, year)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := db.Sync2(new(MinuteCoverage)); err != nil {
		return nil, err
	}
	var c MinuteCoverage
	has, err := db.ID(1).Get(&c)
	if err != nil || !has {
		return nil, err
	}
	return &c, nil
}

func (s *SQLiteStore) saveMinuteCoverage(code Code, year int, writeFrom string, ks []*KlineMinute, coverage MinuteCoverage) error {
	if len(ks) == 0 {
		return nil
	}
	if err := validateMinutes(year, coverage.From, ks); err != nil {
		return err
	}
	db, release, err := s.openMinute(code, year)
	if err != nil {
		return err
	}
	defer release()
	if err := db.Sync2(new(MinuteCoverage)); err != nil {
		return err
	}
	return db.SessionFunc(func(session *xorm.Session) error {
		// 继续使用现有删尾重插规则，保留未返回的本地历史前缀。
		if err := writeMin(session, writeFrom, ks); err != nil {
			return err
		}
		var old MinuteCoverage
		has, err := session.ID(1).Get(&old)
		if err != nil {
			return err
		}
		if has && old.TradeFallback == coverage.TradeFallback &&
			(!coverage.TradeFallback || (old.Through >= coverage.From && coverage.Through >= old.From)) {
			coverage.From = min(old.From, coverage.From)
			coverage.Through = max(old.Through, coverage.Through)
		}
		if _, err := session.ID(1).Delete(new(MinuteCoverage)); err != nil {
			return err
		}
		_, err = session.Insert(&coverage)
		return err
	})
}
