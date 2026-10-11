package pull

import (
	"fmt"
	"path/filepath"
	"time"

	"xorm.io/xorm"
)

// 与行情覆盖标记分开：空历史只暂缓复查，不表示数据完整。
type emptyMinuteScan struct {
	Key       string `xorm:"pk"`
	From      int64
	CheckedAt int64
}

func emptyScanKey(code Code, year int) string {
	return fmt.Sprintf("%s.%s:%d", code.Market, code.Code, year)
}

// MinYearRecentlyEmpty 检查该历史年份是否在最近 7 天内成功扫描为空。
// 更早的起始日期或时钟回退不会复用缓存；不创建空行情库。
func (s *Service) MinYearRecentlyEmpty(code Code, year int, from int64, now time.Time) (bool, error) {
	file := filepath.Join(s.cfg.Dir, "minute-scans.db")
	if !exists(file) {
		return false, nil
	}
	db, release, err := s.engine(file, new(emptyMinuteScan))
	if err != nil {
		return false, err
	}
	defer release()
	var scan emptyMinuteScan
	has, err := db.ID(emptyScanKey(code, year)).Get(&scan)
	age := now.Sub(time.Unix(scan.CheckedAt, 0))
	return has && scan.From <= from && age >= 0 && age < 7*24*time.Hour, err
}

// SaveMinEmptyScan 记录一次成功扫描的空历史范围；调用方不得用它缓存请求失败。
// 内置市场仅在原生扫描返回其他年份的有效数据时调用，不用于分笔兜底。
func (s *Service) SaveMinEmptyScan(code Code, year int, from int64, now time.Time) error {
	db, release, err := s.engine(filepath.Join(s.cfg.Dir, "minute-scans.db"), new(emptyMinuteScan))
	if err != nil {
		return err
	}
	defer release()
	scan := &emptyMinuteScan{Key: emptyScanKey(code, year), From: from, CheckedAt: now.Unix()}
	return db.SessionFunc(func(session *xorm.Session) error {
		if _, err := session.ID(scan.Key).Delete(new(emptyMinuteScan)); err != nil {
			return err
		}
		_, err := session.Insert(scan)
		return err
	})
}
