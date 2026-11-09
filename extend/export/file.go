package export

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"

	"github.com/injoyai/conv"
)

func writeCSV(filename string, rows [][]any) (err error) {
	if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
		return err
	}
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	if _, err := f.WriteString("\xEF\xBB\xBF"); err != nil {
		return err
	}
	w := csv.NewWriter(f)
	for _, row := range rows {
		if err := w.Write(conv.Strings(row)); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
