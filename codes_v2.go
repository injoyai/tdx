package tdx

import (
	"errors"
	"iter"
	"os"
	"path/filepath"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/types"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/internal/gbbq"
	"github.com/injoyai/tdx/internal/xorms"
	"github.com/injoyai/tdx/protocol"
	"github.com/robfig/cron/v3"
	"xorm.io/xorm"
)

type Codes2Option func(*Codes2)

func WithDBFilename(filename string) Codes2Option {
	return func(c *Codes2) {
		c.dbFilename = filename
	}
}

func WithTempDir(dir string) Codes2Option {
	return func(c *Codes2) {
		c.tempDir = dir
	}
}

func WithSpec(spec string) Codes2Option {
	return func(c *Codes2) {
		c.spec = spec
	}
}

func WithUpdateKey(key string) Codes2Option {
	return func(c *Codes2) {
		c.updateKey = key
	}
}

func WithRetry(retry int) Codes2Option {
	return func(c *Codes2) {
		c.retry = retry
	}
}

func WithClient(c *Client) Codes2Option {
	return func(cs *Codes2) {
		cs.c = c
	}
}

func WithDial(dial ios.DialFunc, op ...client.Option) Codes2Option {
	return func(c *Codes2) {
		c.dial = dial
		c.dialOption = op
	}
}

func WithDialOption(op ...client.Option) Codes2Option {
	return func(c *Codes2) {
		c.dialOption = op
	}
}

func NewCodes2(op ...Codes2Option) (*Codes2, error) {
	cs := &Codes2{
		dbFilename: filepath.Join(DefaultDatabaseDir, "codes2.db"),
		tempDir:    filepath.Join(DefaultDataDir, "temp"),
		spec:       "10 0 9 * * *",
		updateKey:  "codes",
		retry:      3,
		dial:       NewRangeDial(Hosts),
		dialOption: nil,
		m:          maps.NewGeneric[string, *CodeModel](),
	}

	for _, o := range op {
		o(cs)
	}

	os.MkdirAll(cs.tempDir, 0777)

	var err error

	// 初始化连接
	if cs.c == nil {
		cs.c, err = DialWith(cs.dial, cs.dialOption...)
		if err != nil {
			return nil, err
		}
	}

	// 初始化数据库
	cs.db, err = xorms.NewSqlite(cs.dbFilename)
	if err != nil {
		return nil, err
	}
	if err = cs.db.Sync2(new(CodeModel), new(UpdateModel)); err != nil {
		return nil, err
	}

	// 立即更新
	err = cs.Update()
	if err != nil {
		return nil, err
	}

	// 定时更新
	cr := cron.New(cron.WithSeconds())
	_, err = cr.AddFunc(cs.spec, func() {
		for i := 0; i < 3; i++ {
			if err := cs.Update(); err != nil {
				logs.Err(err)
				<-time.After(time.Minute * 5)
			} else {
				break
			}
		}
	})
	if err != nil {
		return nil, err
	}

	cr.Start()

	return cs, nil
}

var _ ICodes = &Codes2{}

type Codes2 struct {
	dbFilename string          //数据库文件
	tempDir    string          //临时目录
	spec       string          //定时规则
	updateKey  string          //标识
	retry      int             //重试次数
	dial       ios.DialFunc    //连接
	dialOption []client.Option //

	/*
		内部字段
	*/

	c       *Client                           //
	db      *xorms.Engine                     //
	stocks  types.List[*CodeModel]            //股票缓存
	etfs    types.List[*CodeModel]            //etf缓存
	indexes types.List[*CodeModel]            //指数缓存
	all     types.List[*CodeModel]            //全部缓存
	m       *maps.Generic[string, *CodeModel] //缓存
}

func (this *Codes2) Get(code string) *CodeModel {
	v, _ := this.m.Get(code)
	return v
}

func (this *Codes2) Iter() iter.Seq2[string, *CodeModel] {
	return func(yield func(string, *CodeModel) bool) {
		for _, code := range this.all {
			if !yield(code.FullCode(), code) {
				break
			}
		}
	}
}

func (this *Codes2) GetName(code string) string {
	v, _ := this.m.Get(code)
	if v == nil {
		return "未知"
	}
	return v.Name
}

func (this *Codes2) GetStocks(limit ...int) CodeModels {
	size := conv.Default(this.stocks.Len(), limit...)
	return CodeModels(this.stocks.Limit(size))
}

func (this *Codes2) GetStockCodes(limit ...int) []string {
	return this.GetStocks(limit...).Codes()
}

func (this *Codes2) GetETFs(limit ...int) CodeModels {
	size := conv.Default(this.etfs.Len(), limit...)
	return CodeModels(this.etfs.Limit(size))
}

func (this *Codes2) GetETFCodes(limit ...int) []string {
	return this.GetETFs(limit...).Codes()
}

func (this *Codes2) GetIndexes(limit ...int) CodeModels {
	size := conv.Default(this.etfs.Len(), limit...)
	return CodeModels(this.indexes.Limit(size))
}

func (this *Codes2) GetIndexCodes(limit ...int) []string {
	return this.GetIndexes(limit...).Codes()
}

func (this *Codes2) updated() (bool, error) {
	update := new(UpdateModel)
	{ //查询或者插入一条数据
		has, err := this.db.Where("`Key`=?", this.updateKey).Get(update)
		if err != nil {
			return true, err
		} else if !has {
			update.Key = this.updateKey
			if _, err = this.db.Insert(update); err != nil {
				return true, err
			}
			return false, nil
		}
	}
	{ //判断是否更新过,更新过则不更新
		now := time.Now()
		node := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
		updateTime := time.Unix(update.Time, 0)
		if now.Sub(node) > 0 {
			//当前时间在9点之后,且更新时间在9点之前,需要更新
			if updateTime.Sub(node) < 0 {
				return false, nil
			}
		} else {
			//当前时间在9点之前,且更新时间在上个节点之前
			if updateTime.Sub(node.Add(time.Hour*24)) < 0 {
				return false, nil
			}
		}
	}
	return true, nil
}

func (this *Codes2) Update() error {

	codes, err := this.update()
	if err != nil {
		return err
	}

	stocks := []*CodeModel(nil)
	etfs := []*CodeModel(nil)
	indexes := []*CodeModel(nil)
	for _, v := range codes {
		fullCode := v.FullCode()
		this.m.Set(fullCode, v)
		switch {
		case protocol.IsStock(fullCode):
			stocks = append(stocks, v)
		case protocol.IsETF(fullCode):
			etfs = append(etfs, v)
		case protocol.IsIndex(fullCode):
			indexes = append(indexes, v)
		}
	}

	this.stocks = stocks
	this.etfs = etfs
	this.indexes = indexes
	this.all = codes

	return nil
}

// GetCodes 更新股票并返回结果
func (this *Codes2) update() ([]*CodeModel, error) {

	if this.c == nil {
		return nil, errors.New("client is nil")
	}

	//2. 查询数据库所有股票
	list := []*CodeModel(nil)
	if err := this.db.Find(&list); err != nil {
		return nil, err
	}

	//如果更新过,则不更新
	updated, err := this.updated()
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

	//4. 获取gbbq
	ss, err := gbbq.DownloadAndDecode(this.tempDir)
	if err != nil {
		logs.Err(err)
		return nil, err
	}

	mStock := map[string]gbbq.Stock{}
	for _, v := range ss {
		mStock[protocol.AddPrefix(v.Code)] = v
	}

	//5. 赋值流通股和总股本
	for _, v := range insert {
		if protocol.IsStock(v.FullCode()) {
			v.FloatStock, v.TotalStock = ss.GetStock(v.Code)
		}
	}
	for _, v := range update {
		if stock, ok := mStock[v.FullCode()]; ok {
			v.FloatStock = stock.Float
			v.TotalStock = stock.Total
		}
	}

	//6. 插入或者更新数据库
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

	//更新时间
	_, err = this.db.Where("`Key`=?", this.updateKey).Update(&UpdateModel{Time: time.Now().Unix()})
	return list, err
}
