package pull

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 旧库没有记录时间口径，不能仅凭当前机器的时区推断并自动升级。
func checkTimezone(dir string) error {
	const zone = "UTC+08:00\n"
	marker := filepath.Join(dir, ".storage-timezone")
	if bs, err := os.ReadFile(marker); err == nil {
		if string(bs) != zone {
			return fmt.Errorf("pull: 存储时区标记不兼容，请使用新的数据目录或先迁移旧库")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".db") || strings.HasSuffix(path, ".db-wal") || strings.HasSuffix(path, ".db-shm")) {
			return fmt.Errorf("pull: 目录存在未标记时区的旧库，请使用新目录；旧库须单独迁移后再使用")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if errors.Is(err, os.ErrExist) {
		// 另一个新服务可能刚完成相同目录的初始化。
		return checkTimezone(dir)
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(zone)
	return errors.Join(err, f.Close())
}
