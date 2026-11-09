package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/injoyai/tdx/v2/extend/httpserver"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/metadata/httpclient"
	"github.com/injoyai/tdx/v2/extend/model"
)

type handlerTransport struct{ http.Handler }

func (h handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Result(), nil
}

func TestMetadataHTTPPreservesCompleteSnapshot(t *testing.T) {
	rows := model.CodeModels{
		{Exchange: "sh", Code: "000001", Name: "指数"},
		{Exchange: "sh", Code: "600000", Name: "股票"},
		{Exchange: "sh", Code: "510300", Name: "基金"},
		{Exchange: "sh", Code: "113001", Name: "可转债"},
		{Exchange: "sh", Code: "019001", Name: "其他债券"},
	}
	index, err := metadata.NewCodes(rows)
	if err != nil {
		t.Fatal(err)
	}
	source := httpclient.New("http://offline.invalid", &http.Client{
		Transport: handlerTransport{httpserver.CodesHandler(index, nil)},
	})
	got, err := source.FetchCodes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Snapshot(), rows) {
		t.Fatal("HTTP roundtrip lost securities, fields or order", got.Snapshot())
	}
	for _, row := range rows {
		if !reflect.DeepEqual(got.Get(row.FullCode()), row) {
			t.Fatal("security unavailable after HTTP roundtrip", row.FullCode())
		}
	}
	if err := index.Update(nil); err != nil {
		t.Error(err)
	}
	if len(got.Snapshot()) != len(rows) {
		t.Fatal("published query shares mutable source state")
	}
}
