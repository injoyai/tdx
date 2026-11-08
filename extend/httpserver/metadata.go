package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
)

// CodesSource 是 HTTP 路由所需的证券分类查询能力。
type CodesSource interface {
	GetStocks(...int) model.CodeModels
	GetETFs(...int) model.CodeModels
	GetIndexes(...int) model.CodeModels
}

// ListenCodesAndGbbqHTTP serves caller-owned snapshots; it creates no caches or background refresh jobs.
func ListenCodesAndGbbqHTTP(port int, code CodesSource, equity *metadata.Gbbq) error {
	if missingCodes(code) || equity == nil {
		return errors.New("httpserver: metadata snapshots are required")
	}
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), codesHandler(code, equity))
}

func ListenCodesHTTP(port int, code CodesSource) error {
	if missingCodes(code) {
		return errors.New("httpserver: code snapshot is required")
	}
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), codesHandler(code, nil))
}

func missingCodes(code CodesSource) bool {
	return code == nil || (reflect.ValueOf(code).Kind() == reflect.Pointer && reflect.ValueOf(code).IsNil())
}

// CodesHandler exposes the current caller-owned snapshots for mounting on an existing server.
func CodesHandler(code CodesSource, equity *metadata.Gbbq) http.Handler {
	return codesHandler(code, equity)
}

func codesHandler(code CodesSource, equity *metadata.Gbbq) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missingCodes(code) {
			http.Error(w, "code snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		var data any
		switch r.URL.Path {
		case "/codes":
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
