// Package sqlite 提供 SQLite 连接和 tdx 行情持久化实现。
package sqlite

import (
	"os"
	"path/filepath"

	_ "github.com/glebarez/go-sqlite"
	"xorm.io/xorm"
	"xorm.io/xorm/names"
)

// Open 创建父目录并打开引擎；使用原样字段名、单连接，调用方负责 Close。
// 返回后可由调用方调整 mapper、时区和连接池设置。
func Open(filename string) (*xorm.Engine, error) {
	if err := os.MkdirAll(filepath.Dir(filename), 0777); err != nil {
		return nil, err
	}
	db, err := xorm.NewEngine("sqlite", filename)
	if err != nil {
		return nil, err
	}
	db.SetMapper(names.SameMapper{})
	db.SetMaxOpenConns(1)
	return db, nil
}
