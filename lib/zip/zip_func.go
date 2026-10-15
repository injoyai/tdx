package zip

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Encode 压缩文件
// @filePath,文件路径
// @zipName,压缩名称
func Encode(filePath, zipName string) (err error) {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	source, err := file.Stat()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(zipName), os.ModePerm); err != nil {
		return err
	}
	// 检查已打开的目标，避免别名或链接指向源文件时被截断。
	zipFile, err := os.OpenFile(zipName, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, zipFile.Close()) }()
	target, err := zipFile.Stat()
	if err != nil {
		return err
	}
	if os.SameFile(source, target) {
		return fmt.Errorf("zip: source and destination are the same file")
	}
	if err := zipFile.Truncate(0); err != nil {
		return err
	}
	zipWriter := zip.NewWriter(zipFile)
	defer func() { err = errors.Join(err, zipWriter.Close()) }()
	return compareZip(file, zipWriter, "", !strings.HasSuffix(filePath, "/"))
}

// 压缩文件
func compareZip(file *os.File, zipWriter *zip.Writer, prefix string, join bool) error {
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		return err
	}

	if join {
		header.Name = filepath.Join(prefix, header.Name)
		prefix = filepath.Join(prefix, fileInfo.Name())
		header.Name = strings.ReplaceAll(header.Name, "\\", "/")
		if fileInfo.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate //压缩的关键
		}
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		if !fileInfo.IsDir() {
			_, err = io.Copy(writer, file)
			return err
		}
	}

	fileInfoChildList, err := file.Readdir(-1)
	if err != nil {
		return err
	}

	for _, fileInfoChild := range fileInfoChildList {
		fileChild, err := os.Open(filepath.Join(file.Name(), fileInfoChild.Name()))
		if err != nil {
			return err
		}
		if err := compareZip(fileChild, zipWriter, prefix, true); err != nil {
			return err
		}
	}
	return nil

}

// Decode 解压zip
func Decode(zipName, filePath string) error {
	r, err := zip.OpenReader(zipName)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
		return err
	}
	root, err := os.OpenRoot(filePath)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, k := range r.Reader.File {
		name, err := filepath.Localize(path.Clean(k.Name))
		if err != nil || strings.Contains(k.Name, "\\") {
			return fmt.Errorf("zip: invalid path %q", k.Name)
		}
		if k.FileInfo().IsDir() {
			if err := root.MkdirAll(name, os.ModePerm); err != nil {
				return err
			}
			continue
		}
		if !k.Mode().IsRegular() {
			return fmt.Errorf("zip: unsupported file type %q", k.Name)
		}
		if err := root.MkdirAll(filepath.Dir(name), os.ModePerm); err != nil {
			return err
		}
		if err := func() error {
			f, err := k.Open()
			if err != nil {
				return err
			}
			defer f.Close()
			w, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, f)
			return errors.Join(err, w.Close())
		}(); err != nil {
			return err
		}
	}
	return nil
}
