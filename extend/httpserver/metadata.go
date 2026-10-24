package httpserver

import (
	"fmt"
	"net/http"

	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
)

func ListenCodesAndGbbqHTTP(port int, codesOption []codes.Option, equityOption []gbbq.Option) error {
	code, err := codes.New(codesOption...)
	if err != nil {
		return err
	}
	defer code.Close()
	equity, err := gbbq.New(equityOption...)
	if err != nil {
		return err
	}
	defer equity.Close()
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), codesHandler(code, equity))
}

func ListenCodesHTTP(port int, op ...codes.Option) error {
	code, err := codes.New(op...)
	if err != nil {
		return err
	}
	defer code.Close()
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), codesHandler(code, nil))
}

func codesHandler(code codes.Interface, equity *gbbq.Gbbq) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch r.URL.Path {
		case "/codes", "/all":
			ls := code.GetStocks()
			ls = append(ls, code.GetETFs()...)
			data = append(ls, code.GetIndexes()...)
		case "/stocks":
			data = code.GetStocks()
		case "/etfs":
			data = code.GetETFs()
		case "/indexes":
			data = code.GetIndexes()
		case "/gbbqs":
			if equity == nil {
				http.NotFound(w, r)
				return
			}
			data = equity.All()
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(conv.Bytes(data))
	})
}
