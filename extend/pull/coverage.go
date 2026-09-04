package pull

import (
	"fmt"
	"time"

	"xorm.io/xorm"
)

// MinuteCoverage 记录已处理的请求范围；不保证服务器提供的数据没有缺笔。
// TradeFallback 区分旧的原生扫描标记，Through 是已处理范围的右开端点。
type MinuteCoverage struct {
	ID            int `xorm:"pk"`
	From          int64
	Through       int64
	TradeFallback bool
}

// TableName 保持已有历史库的表名。
func (MinuteCoverage) TableName() string { return "minuteCoverage" }

// QueryMinCoverage 查询分钟覆盖记录；旧库会补齐字段，缺少记录时返回 nil。
func (s *Service) QueryMinCoverage(code Code, year int) (*MinuteCoverage, error) {
	if !s.MinExists(code, year) {
		return nil, nil
	}
	db, release, err := s.openMin(code, year)
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

// MinYearComplete 保留原生分钟扫描语义；分笔兜底还需检查 TradeFallback 与 Through。
func (s *Service) MinYearComplete(code Code, year int, from int64) (bool, error) {
	c, err := s.QueryMinCoverage(code, year)
	through := time.Date(year+1, 1, 1, 0, 0, 0, 0, s.Start().Location()).Unix()
	return c != nil && c.From <= from && (!c.TradeFallback || c.Through >= through), err
}

// SaveMinComplete 将原生历史扫描数据及完成标记原子提交，空数据不写标记。
func (s *Service) SaveMinComplete(code Code, year int, from int64, ks []*KlineMinute) error {
	return s.saveMinCoverage(code, year, ks, MinuteCoverage{ID: 1, From: from})
}

// SaveMinFallbackComplete 提交原生与合成数据，以及 [from,through) 已检查标记。
// 调用方须先处理所有缺失交易日，并将需保留的本地尾部数据合并到 ks。
func (s *Service) SaveMinFallbackComplete(code Code, year int, from, through int64, ks []*KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	if through <= from {
		return fmt.Errorf("pull: 无效分钟覆盖区间 %d..%d", from, through)
	}
	return s.saveMinCoverage(code, year, ks, MinuteCoverage{
		ID: 1, From: from, Through: through, TradeFallback: true,
	})
}

func (s *Service) saveMinCoverage(code Code, year int, ks []*KlineMinute, coverage MinuteCoverage) error {
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
