package sqlite

import (
	"path/filepath"
	"strings"

	"github.com/injoyai/tdx/v2/extend/model"
)

// DayFile 该代码的日线 sqlite 文件路径。
// 布局：{dir}/{地区}/{资产}/day/{code}.db（两级目录含市场信息，文件名用原始代码）。
func dayFile(c model.Code, dir string) string {
	return filepath.Join(dir, c.Market.String(), "day", strings.ReplaceAll(c.Code, "/", "-")+".db")
}

// MinuteFile 该代码指定年份的1分钟线 sqlite 文件路径。
// 布局：{dir}/{地区}/{资产}/min/{code}/{code}-{year}.db（子目录与文件名均用原始代码，
// 便于按代码单独检索/拷贝；期货代码中的 "/" 替换为 "-"）。
func minuteFile(c model.Code, dir string, year int) string {
	y := yearToString(year)
	name := strings.ReplaceAll(c.Code, "/", "-")
	return filepath.Join(dir, c.Market.String(), "min", name, name+"-"+y+".db")
}

func yearToString(year int) string {
	buf := []byte("0000")
	buf[0] = byte('0' + (year/1000)%10)
	buf[1] = byte('0' + (year/100)%10)
	buf[2] = byte('0' + (year/10)%10)
	buf[3] = byte('0' + year%10)
	return string(buf)
}
