package extend

import (
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/robfig/cron/v3"
)

func ListenCodesHTTP(port int, op ...tdx.Codes2Option) error {
	code, err := tdx.NewCodes2(op...)
	if err != nil {
		return nil
	}
	succ := func(w http.ResponseWriter, data any) {
		w.WriteHeader(http.StatusOK)
		w.Write(conv.Bytes(data))
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", port), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/all":

		case "/stocks":
			succ(w, code.GetStocks())
		case "/etfs":
			succ(w, code.GetETFs())
		case "/indexes":
			succ(w, code.GetIndexes())
		default:
			http.NotFound(w, r)
		}
	}))
}

func DialCodesHTTP(address string) (c *CodesHTTP, err error) {
	c = &CodesHTTP{address: address}
	cr := cron.New(cron.WithSeconds())
	_, err = cr.AddFunc("0 20 9 * * *", func() { logs.PrintErr(c.Update()) })
	if err != nil {
		return
	}
	err = c.Update()
	if err != nil {
		return
	}
	cr.Start()
	return c, nil
}

type CodesHTTP struct {
	address string
	stocks  tdx.CodeModels
	etfs    tdx.CodeModels
	indexes tdx.CodeModels
	m       maps.Generic[string, *tdx.CodeModel]
}

func (this *CodesHTTP) Iter() iter.Seq2[string, *tdx.CodeModel] {
	return func(yield func(string, *tdx.CodeModel) bool) {
		for _, v := range this.stocks {
			if !yield(v.FullCode(), v) {
				return
			}
		}
		for _, v := range this.etfs {
			if !yield(v.FullCode(), v) {
				return
			}
		}
		for _, v := range this.indexes {
			if !yield(v.FullCode(), v) {
				return
			}
		}
	}
}

func (this *CodesHTTP) Get(code string) *tdx.CodeModel {
	return this.m.MustGet(code)
}

func (this *CodesHTTP) GetName(code string) string {
	v := this.m.MustGet(code)
	if v != nil {
		return v.Name
	}
	return ""
}

func (this *CodesHTTP) GetStocks(limit ...int) tdx.CodeModels {
	return this.stocks
}

func (this *CodesHTTP) GetStockCodes(limit ...int) []string {
	return this.stocks.Codes()
}

func (this *CodesHTTP) GetETFs(limit ...int) tdx.CodeModels {
	return this.etfs
}

func (this *CodesHTTP) GetETFCodes(limit ...int) []string {
	return this.etfs.Codes()
}

func (this *CodesHTTP) GetIndexes(limits ...int) tdx.CodeModels {
	return this.indexes
}

func (this *CodesHTTP) GetIndexCodes(limits ...int) []string {
	return this.indexes.Codes()
}

func (this *CodesHTTP) Update() (err error) {
	this.stocks, err = this.getList("/stocks")
	if err != nil {
		return
	}
	for _, v := range this.stocks {
		this.m.Set(v.FullCode(), v)
	}
	this.etfs, err = this.getList("/etfs")
	if err != nil {
		return
	}
	for _, v := range this.etfs {
		this.m.Set(v.FullCode(), v)
	}
	this.indexes, err = this.getList("/indexes")
	if err != nil {
		return
	}
	for _, v := range this.indexes {
		this.m.Set(v.FullCode(), v)
	}
	return
}

func (this *CodesHTTP) getList(path string) (tdx.CodeModels, error) {
	resp, err := http.DefaultClient.Get(this.address + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http code:%d", resp.StatusCode)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ls := tdx.CodeModels{}
	err = json.Unmarshal(bs, &ls)
	return ls, err
}
