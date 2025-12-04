package main

import (
	"strings"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
)

func main() {
	cs, err := tdx.NewCodes()
	logs.PanicErr(err)

	ls := cs.GetETFCodes()

	shNumber := 0
	szNumber := 0
	for _, v := range ls {
		switch {
		case strings.HasPrefix(v, "sh"):
			shNumber++
		case strings.HasPrefix(v, "sz"):
			szNumber++
		}
	}

	logs.Debug("sh:", shNumber)
	logs.Debug("sz:", szNumber)
}
