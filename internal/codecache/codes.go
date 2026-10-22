// Package codecache 提供标准行情与扩展代码服务共享的纯内存代码表。
package codecache

import (
	"iter"
	"math"
	"sync"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2/protocol"
)

// Interface 是不触发网络访问的代码表读取接口。
type Interface interface {
	Iter() iter.Seq2[string, *CodeModel]
	Get(code string) *CodeModel
	GetName(code string) string
	GetStocks(limit ...int) CodeModels
	GetStockCodes(limit ...int) []string
	GetETFs(limit ...int) CodeModels
	GetETFCodes(limit ...int) []string
	GetIndexes(limit ...int) CodeModels
	GetIndexCodes(limit ...int) []string
}

type CodeModel struct {
	ID        int64   `json:"id"`                      //主键
	Name      string  `json:"name"`                    //名称,有时候名称会变,例STxxx
	Code      string  `json:"code" xorm:"index"`       //代码
	Exchange  string  `json:"exchange" xorm:"index"`   //交易所
	Multiple  uint16  `json:"multiple"`                //倍数
	Decimal   int8    `json:"decimal"`                 //小数位
	LastPrice float64 `json:"lastPrice"`               //昨收价格
	EditDate  int64   `json:"editDate" xorm:"updated"` //修改时间
	InDate    int64   `json:"inDate" xorm:"created"`   //创建时间
}

func (*CodeModel) TableName() string {
	return "codes"
}

// FullCode 获取完整代码 sz000001
func (c *CodeModel) FullCode() string {
	return c.Exchange + c.Code
}

func (c *CodeModel) Price(p protocol.Price) protocol.Price {
	return protocol.Price(float64(p) * math.Pow10(int(2-c.Decimal)))
}

type CodeModels []*CodeModel

func (cs CodeModels) Codes() []string {
	codes := make([]string, len(cs))
	for i, v := range cs {
		codes[i] = v.FullCode()
	}
	return codes
}

var _ Interface = &Base{}

func NewBase() *Base {
	c := &Base{}
	c.Update(nil)
	return c
}

type Base struct {
	list []*CodeModel
	m    map[string]*CodeModel
	mu   sync.Mutex
}

func (cb *Base) Update(ls []*CodeModel) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.list = append([]*CodeModel(nil), ls...)
	cb.m = make(map[string]*CodeModel)
	for _, v := range ls {
		cb.m[v.FullCode()] = v
	}
}

func (cb *Base) Iter() iter.Seq2[string, *CodeModel] {
	return func(yield func(string, *CodeModel) bool) {
		cb.mu.Lock()
		list := cb.list
		cb.mu.Unlock()
		for _, v := range list {
			if !yield(v.FullCode(), v) {
				return
			}
		}
	}
}

func (cb *Base) Get(code string) *CodeModel {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.m[code]
}

// GetName 获取股票名称
func (cb *Base) GetName(code string) string {
	c := cb.Get(code)
	if c != nil {
		return c.Name
	}
	return ""
}

// GetStocks 获取股票代码,sh6xxx sz0xx sz30xx
func (cb *Base) GetStocks(limits ...int) CodeModels {
	return cb.filter(protocol.IsStock, limits...)
}

// GetStockCodes 获取股票代码,sh6xxx sz0xx sz30xx
func (cb *Base) GetStockCodes(limits ...int) []string {
	return cb.GetStocks(limits...).Codes()
}

// GetETFs 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (cb *Base) GetETFs(limits ...int) CodeModels {
	return cb.filter(protocol.IsETF, limits...)
}

// GetETFCodes 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (cb *Base) GetETFCodes(limits ...int) []string {
	return cb.GetETFs(limits...).Codes()
}

// GetIndexes 获取指数代码。
func (cb *Base) GetIndexes(limits ...int) CodeModels {
	return cb.filter(protocol.IsIndex, limits...)
}

// GetIndexCodes 获取完整指数代码。
func (cb *Base) GetIndexCodes(limits ...int) []string {
	return cb.GetIndexes(limits...).Codes()
}

func (cb *Base) filter(match func(string) bool, limits ...int) CodeModels {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	limit := conv.Default(-1, limits...)
	var ls CodeModels
	for _, m := range cb.list {
		if match(m.FullCode()) {
			ls = append(ls, m)
		}
		if limit > 0 && len(ls) >= limit {
			break
		}
	}
	return ls
}
