package httpserver

import (
	"net/http"
	"reflect"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// CodesSource 提供一次取得完整证券快照的能力；返回数据由当前请求独立持有。
type CodesSource interface {
	Snapshot() model.CodeModels
}

func missingCodes(code CodesSource) bool {
	return code == nil || (reflect.ValueOf(code).Kind() == reflect.Pointer && reflect.ValueOf(code).IsNil())
}

// CodesHandler exposes the current caller-owned snapshots for mounting on an existing server.
func CodesHandler(code CodesSource, equity *metadata.Gbbq) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if missingCodes(code) {
			http.Error(w, "code snapshot unavailable", http.StatusServiceUnavailable)
			return
		}
		var data any
		var match func(string) bool
		switch r.URL.Path {
		case "/codes":
			data = code.Snapshot()
		case "/stocks":
			match = protocol.IsStock
		case "/etfs":
			match = protocol.IsETF
		case "/indexes":
			match = protocol.IsIndex
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
		if match != nil {
			rows := model.CodeModels{}
			for _, row := range code.Snapshot() {
				if row != nil && match(row.FullCode()) {
					rows = append(rows, row)
				}
			}
			data = rows
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(conv.Bytes(data))
	})
}
