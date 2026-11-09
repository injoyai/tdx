package metadata

import (
	"iter"
	"sync"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// NewCodes 校验并复制完整记录，创建代码索引；传 nil 可创建空索引。
func NewCodes(records []*model.CodeModel) (*Codes, error) {
	c := &Codes{}
	if err := c.Update(records); err != nil {
		return nil, err
	}
	return c, nil
}

// Codes 在内存中维护一份可原子替换的证券快照。
// 构造、更新和读取均复制记录，避免调用方修改内部数据。
type Codes struct {
	list []*model.CodeModel
	m    map[string]*model.CodeModel
	mu   sync.Mutex
}

// Update 完整校验后原子替换快照；失败保留旧记录。
func (c *Codes) Update(ls []*model.CodeModel) error {
	if err := model.ValidateCodes(ls); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = make([]*model.CodeModel, 0, len(ls))
	c.m = make(map[string]*model.CodeModel)
	for _, row := range ls {
		v := *row
		c.list = append(c.list, &v)
		c.m[v.FullCode()] = &v
	}
	return nil
}

// Snapshot 在一次加锁内复制全部证券，包含未归入股票、ETF 或指数的品种。
func (c *Codes) Snapshot() model.CodeModels {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := make(model.CodeModels, 0, len(c.list))
	for _, row := range c.list {
		copy := *row
		rows = append(rows, &copy)
	}
	return rows
}

func (c *Codes) Iter() iter.Seq2[string, *model.CodeModel] {
	return func(yield func(string, *model.CodeModel) bool) {
		c.mu.Lock()
		list := c.list
		c.mu.Unlock()
		for _, v := range list {
			copy := *v
			if !yield(v.FullCode(), &copy) {
				return
			}
		}
	}
}

func (c *Codes) Get(code string) *model.CodeModel {
	key, err := model.StandardCode(code)
	if err != nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if v := c.m[key]; v != nil {
		copy := *v
		return &copy
	}
	return nil
}

// GetName 获取股票名称
func (c *Codes) GetName(code string) string {
	record := c.Get(code)
	if record != nil {
		return record.Name
	}
	return ""
}

// GetStocks 获取股票代码,sh6xxx sz0xx sz30xx
func (c *Codes) GetStocks(limits ...int) model.CodeModels {
	return c.filter(protocol.IsStock, limits...)
}

// GetStockCodes 获取股票代码,sh6xxx sz0xx sz30xx
func (c *Codes) GetStockCodes(limits ...int) []string {
	return c.GetStocks(limits...).Codes()
}

// GetETFs 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (c *Codes) GetETFs(limits ...int) model.CodeModels {
	return c.filter(protocol.IsETF, limits...)
}

// GetETFCodes 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (c *Codes) GetETFCodes(limits ...int) []string {
	return c.GetETFs(limits...).Codes()
}

// GetIndexes 获取指数代码。
func (c *Codes) GetIndexes(limits ...int) model.CodeModels {
	return c.filter(protocol.IsIndex, limits...)
}

// GetIndexCodes 获取完整指数代码。
func (c *Codes) GetIndexCodes(limits ...int) []string {
	return c.GetIndexes(limits...).Codes()
}

func (c *Codes) filter(match func(string) bool, limits ...int) model.CodeModels {
	c.mu.Lock()
	defer c.mu.Unlock()
	limit := conv.Default(-1, limits...)
	var ls model.CodeModels
	for _, m := range c.list {
		if match(m.FullCode()) {
			copy := *m
			ls = append(ls, &copy)
		}
		if limit > 0 && len(ls) >= limit {
			break
		}
	}
	return ls
}
