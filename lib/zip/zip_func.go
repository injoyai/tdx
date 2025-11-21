package zip

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Encode 压缩文件
// @filePath,文件路径
// @zipName,压缩名称
func Encode(filePath, zipName string) error {

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	os.MkdirAll(filepath.Dir(zipName), os.ModePerm)

	zipFile, err := os.Create(zipName)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

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
	for _, k := range r.Reader.File {
		if k.FileInfo().IsDir() {
			if err := os.MkdirAll(filepath.Join(filePath, k.Name), os.ModePerm); err != nil {
				return err
			}
		} else {
			err := func() error {
				f, err := k.Open()
				if err != nil {
					return err
				}
				defer f.Close()
				w, err := os.Create(filepath.Join(filePath, k.Name))
				if err != nil {
					return err
				}
				defer w.Close()
				_, err = io.Copy(w, f)
				return err
			}()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
