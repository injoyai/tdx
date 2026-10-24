package pull

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 源时间与旧 Unix 主键格式隔离，不自动迁移或修改旧库。
func checkTimeFormat(dir string) error {
	const format = "source-time-v1\n"
	marker := filepath.Join(dir, ".time-format")
	if _, err := os.Stat(filepath.Join(dir, ".storage-timezone")); err == nil {
		return fmt.Errorf("pull: 目录使用旧 Unix 时间格式，请使用新的数据目录")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if bs, err := os.ReadFile(marker); err == nil {
		if string(bs) != format {
			return fmt.Errorf("pull: 存储时间格式不兼容，请使用新的数据目录")
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
			return fmt.Errorf("pull: 目录存在旧数据库，请使用新的数据目录（不支持旧数据格式）")
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
		return checkTimeFormat(dir)
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(format)
	return errors.Join(err, f.Close())
}
