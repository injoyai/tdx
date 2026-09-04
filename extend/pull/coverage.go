package pull

import "xorm.io/xorm"

// minuteCoverage 与历史年份 K 线同事务提交；旧库或失败留下的库无标记，需重新补拉。
type minuteCoverage struct {
	ID   int `xorm:"pk"`
	From int64
}

// MinYearComplete 判断历史年份是否已成功扫描 from 到年末；文件存在不代表完成。
func (s *Service) MinYearComplete(code Code, year int, from int64) (bool, error) {
	if !s.MinExists(code, year) {
		return false, nil
	}
	db, release, err := s.openMin(code, year)
	if err != nil {
		return false, err
	}
	defer release()
	if err := db.Sync2(new(minuteCoverage)); err != nil {
		return false, err
	}
	var c minuteCoverage
	has, err := db.ID(1).Get(&c)
	return has && c.From <= from, err
}

// SaveMinComplete 将历史年份数据及完成标记原子提交。
// 调用者必须先成功扫描 from 到年末；空数据不写标记，协议上限错误不得调用此方法。
// 只替换本次实际返回的时间段，保留服务器已不再提供的本地历史前缀。
func (s *Service) SaveMinComplete(code Code, year int, from int64, ks []*KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	writeFrom := ks[0].Unix
	for _, k := range ks {
		if k.Unix < writeFrom {
			writeFrom = k.Unix
		}
	}
	db, release, err := s.openMin(code, year)
	if err != nil {
		return err
	}
	defer release()
	if err := db.Sync2(new(minuteCoverage)); err != nil {
		return err
	}
	return db.SessionFunc(func(session *xorm.Session) error {
		if err := writeMin(session, writeFrom, ks); err != nil {
			return err
		}
		var old minuteCoverage
		has, err := session.ID(1).Get(&old)
		if err != nil {
			return err
		}
		if has && old.From < from {
			from = old.From
		}
		if _, err := session.ID(1).Delete(new(minuteCoverage)); err != nil {
			return err
		}
		_, err = session.Insert(&minuteCoverage{ID: 1, From: from})
		return err
	})
}
