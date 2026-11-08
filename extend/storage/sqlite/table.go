package sqlite

import (
	"errors"

	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

// ensureTable 只初始化缺失的 SQLite 表及索引，不自动迁移已有表。
func ensureTable(db *xorm.Engine, record any) error {
	if db.Dialect().URI().DBType != schemas.SQLITE {
		return errors.New("内置 SQLStore 仅支持 SQLite；其他数据库请提供自定义 Store")
	}
	exists, err := db.IsTableExist(record)
	if err != nil || exists {
		return err
	}
	session := db.NewSession()
	defer session.Close()
	return session.Sync(record)
}
