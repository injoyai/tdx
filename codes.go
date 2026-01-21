package tdx

import (
	"errors"
	"iter"
	"math"
	"path/filepath"
	"sync"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
	"xorm.io/xorm"
)

type (
	CodesOption func(*Codes)

	DialCodesFunc func(c *Client) (ICodes, error)
)

type ICodes interface {
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

// DefaultCodes 增加单例,部分数据需要通过Codes里面的信息计算
var DefaultCodes ICodes

func WithCodesDB(db *xorms.Engine) CodesOption {
	return func(c *Codes) {
		c.db = db
	}
}

func WithCodesDialDB(dial DialDBFunc) CodesOption {
	return func(c *Codes) {
		c.dialDB = dial
	}
}

func WithCodesSpec(spec string) CodesOption {
	return func(c *Codes) {
		c.spec = spec
	}
}

func WithCodesRetry(retry int) CodesOption {
	return func(c *Codes) {
		c.retry = retry
	}
}

func WithCodesClient(c *Client) CodesOption {
	return func(cs *Codes) {
		cs.c = c
	}
}

func WithCodesDialClient(dial DialClientFunc) CodesOption {
	return func(c *Codes) {
		c.dialClient = dial
	}
}

func WithCodesOption(op ...CodesOption) CodesOption {
	return func(c *Codes) {
		for _, v := range op {
			if v != nil {
				v(c)
			}
		}
	}
}

func NewCodesMysql(dsn string, op ...CodesOption) (*Codes, error) {
	return NewCodes(
		WithCodesDialDB(func() (*xorms.Engine, error) {
			return xorms.NewMysql(dsn)
		}),
		WithCodesOption(op...),
	)
}

func NewCodesSqlite(op ...CodesOption) (*Codes, error) {
	return NewCodes(op...)
}

func NewCodes(op ...CodesOption) (*Codes, error) {
	cs := &Codes{
		spec:      DefaultCodesSpec,
		retry:     DefaultRetry,
		CodesBase: NewCodesBase(),
		updateKey: "codes",
	}

	WithCodesOption(op...)(cs)

	var err error

	// 初始化连接
	if cs.c == nil {
		if cs.dialClient == nil {
			cs.dialClient = func() (*Client, error) { return DialDefault() }
		}
		cs.c, err = cs.dialClient()
		if err != nil {
			return nil, err
		}
	}

	// 初始化数据库
	if cs.db == nil {
		if cs.dialDB == nil {
			cs.dialDB = func() (*xorms.Engine, error) { return xorms.NewSqlite(filepath.Join(DefaultDatabaseDir, "codes.db")) }
		}
		cs.db, err = cs.dialDB()
		if err != nil {
			return nil, err
		}
		if err = cs.db.Sync2(new(CodeModel)); err != nil {
			return nil, err
		}
	}
	cs.updated, err = NewUpdated(cs.db, 9, 0)
	if err != nil {
		return nil, err
	}

	// 立即/定时更新
	err = NewTimer(cs.spec, cs.retry, cs)

	return cs, err
}

var _ ICodes = &Codes{}

type Codes struct {
	spec  string //定时规则
	retry int    //重试次数

	dialDB     DialDBFunc
	dialClient DialClientFunc

	/*
		内部字段
	*/

	c         *Client
	db        *xorms.Engine
	updateKey string
	updated   *Updated

	*CodesBase
}

func (this *Codes) Update() error {
	codes, err := this.update()
	if err != nil {
		return err
	}
	this.CodesBase.Update(codes)
	return nil
}

// GetCodes 更新股票并返回结果
func (this *Codes) update() ([]*CodeModel, error) {

	if this.c == nil {
		return nil, errors.New("client is nil")
	}

	//2. 查询数据库所有股票
	list := []*CodeModel(nil)
	if err := this.db.Find(&list); err != nil {
		return nil, err
	}

	//如果更新过,则不更新
	updated, err := this.updated.Updated(this.updateKey)
	if err == nil && updated {
		return list, nil
	}

	mCode := make(map[string]*CodeModel, len(list))
	for _, v := range list {
		mCode[v.FullCode()] = v
	}

	//3. 从服务器获取所有股票代码
	insert := []*CodeModel(nil)
	update := []*CodeModel(nil)
	for _, exchange := range []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ, protocol.ExchangeBJ} {
		resp, err := this.c.GetCodeAll(exchange)
		if err != nil {
			return nil, err
		}
		for _, v := range resp.List {
			code := &CodeModel{
				Name:      v.Name,
				Code:      v.Code,
				Exchange:  exchange.String(),
				Multiple:  v.Multiple,
				Decimal:   v.Decimal,
				LastPrice: v.LastPrice,
			}
			if val, ok := mCode[exchange.String()+v.Code]; ok {
				if val.Name != v.Name {
					update = append(update, code)
				}
				delete(mCode, exchange.String()+v.Code)
			} else {
				insert = append(insert, code)
				list = append(list, code)
			}
		}
	}

	switch this.db.Dialect().URI().DBType {
	case "mysql":
		// 1️⃣ 清空
		if _, err := this.db.Exec("TRUNCATE TABLE codes"); err != nil {
			return nil, err
		}

		data := append(insert, update...)
		// 2️⃣ 直接批量插入
		batchSize := 3000 // 8000(2m16s) 5000(43s) 3000(11s) 1000(59s)
		for i := 0; i < len(data); i += batchSize {
			end := i + batchSize
			if end > len(data) {
				end = len(data)
			}

			slice := conv.Array(data[i:end])
			if _, err := this.db.Insert(slice); err != nil {
				return nil, err
			}
		}
	default: //"sqlite3":
		//4. 插入或者更新数据库
		err = this.db.SessionFunc(func(session *xorm.Session) error {
			for _, v := range mCode {
				if _, err = session.Where("Exchange=? and Code=? ", v.Exchange, v.Code).Delete(v); err != nil {
					return err
				}
			}
			for _, v := range insert {
				if _, err := session.Insert(v); err != nil {
					return err
				}
			}
			for _, v := range update {
				if _, err = session.Where("Exchange=? and Code=? ", v.Exchange, v.Code).Cols("Name,LastPrice").Update(v); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	//更新时间
	err = this.updated.Update(this.updateKey)
	return list, err
}

/*



 */

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
func (this *CodeModel) FullCode() string {
	return this.Exchange + this.Code
}

func (this *CodeModel) Price(p protocol.Price) protocol.Price {
	return protocol.Price(float64(p) * math.Pow10(int(2-this.Decimal)))
}

type CodeModels []*CodeModel

func (this CodeModels) Codes() []string {
	codes := make([]string, len(this))
	for i, v := range this {
		codes[i] = v.FullCode()
	}
	return codes
}

/*



 */

var _ ICodes = &CodesBase{}

func NewCodesBase() *CodesBase {
	c := &CodesBase{}
	c.Update(nil)
	return c
}

type CodesBase struct {
	list []*CodeModel
	m    map[string]*CodeModel
	mu   sync.Mutex
}

func (this *CodesBase) Update(ls []*CodeModel) {
	this.mu.Lock()
	defer this.mu.Unlock()
	this.list = ls
	this.m = make(map[string]*CodeModel)
	for _, v := range ls {
		this.m[v.FullCode()] = v
	}
}

func (this *CodesBase) Iter() iter.Seq2[string, *CodeModel] {
	return func(yield func(string, *CodeModel) bool) {
		for _, v := range this.list {
			if !yield(v.FullCode(), v) {
				return
			}
		}
	}
}

func (this *CodesBase) Get(code string) *CodeModel {
	this.mu.Lock()
	defer this.mu.Unlock()
	return this.m[code]
}

// GetName 获取股票名称
func (this *CodesBase) GetName(code string) string {
	c := this.Get(code)
	if c != nil {
		return c.Name
	}
	return ""
}

// GetStocks 获取股票代码,sh6xxx sz0xx sz30xx
func (this *CodesBase) GetStocks(limits ...int) CodeModels {
	limit := conv.Default(-1, limits...)
	ls := []*CodeModel(nil)
	for _, m := range this.list {
		code := m.FullCode()
		if protocol.IsStock(code) {
			ls = append(ls, m)
		}
		if limit > 0 && len(ls) >= limit {
			break
		}
	}
	return ls
}

// GetStockCodes 获取股票代码,sh6xxx sz0xx sz30xx
func (this *CodesBase) GetStockCodes(limits ...int) []string {
	return this.GetStocks(limits...).Codes()
}

// GetETFs 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (this *CodesBase) GetETFs(limits ...int) CodeModels {
	limit := conv.Default(-1, limits...)
	ls := []*CodeModel(nil)
	for _, m := range this.list {
		code := m.FullCode()
		if protocol.IsETF(code) {
			ls = append(ls, m)
		}
		if limit > 0 && len(ls) >= limit {
			break
		}
	}
	return ls
}

// GetETFCodes 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (this *CodesBase) GetETFCodes(limits ...int) []string {
	return this.GetETFs(limits...).Codes()
}

// GetIndexes 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (this *CodesBase) GetIndexes(limits ...int) CodeModels {
	limit := conv.Default(-1, limits...)
	ls := []*CodeModel(nil)
	for _, m := range this.list {
		code := m.FullCode()
		if protocol.IsIndex(code) {
			ls = append(ls, m)
		}
		if limit > 0 && len(ls) >= limit {
			break
		}
	}
	return ls
}

// GetIndexCodes 获取基金代码,sz159xxx,sh510xxx,sh511xxx
func (this *CodesBase) GetIndexCodes(limits ...int) []string {
	return this.GetIndexes(limits...).Codes()
}
