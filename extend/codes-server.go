package extend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/all":
			ls := code.GetStocks()
			ls = append(ls, code.GetETFs()...)
			ls = append(ls, code.GetIndexes()...)
			succ(w, ls)
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

func DialCodesHTTP(address string, spec ...string) (c *CodesHTTP, err error) {
	c = &CodesHTTP{address: address}
	cr := cron.New(cron.WithSeconds())
	_spec := conv.Default("0 20 9 * * *", spec...)
	_, err = cr.AddFunc(_spec, func() { logs.PrintErr(c.Update()) })
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
	*tdx.CodesBase
}

func (this *CodesHTTP) Update() error {
	ls, err := this.getList("/all")
	if err != nil {
		return err
	}
	this.CodesBase.Update(ls)
	return nil
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
