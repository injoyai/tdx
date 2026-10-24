// Package extend 保留收益计算及旧入口；新代码应直接导入相应职责子包。
package extend

import (
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/export"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/httpserver"
	"github.com/injoyai/tdx/v2/extend/local"
	"github.com/injoyai/tdx/v2/extend/metadatahttp"
	"github.com/injoyai/tdx/v2/extend/pulltrade"
	"github.com/injoyai/tdx/v2/extend/spider"
	"github.com/injoyai/tdx/v2/protocol"
)

// Deprecated: 使用 local.ReadDay。
func ReadDay(dir, code string) (protocol.Klines, error) { return local.ReadDay(dir, code) }

// Deprecated: 使用 local.ReadMinute1。
func ReadMinute1(dir, code string) (protocol.Klines, error) { return local.ReadMinute1(dir, code) }

// Deprecated: 使用 local.ReadMinute5。
func ReadMinute5(dir, code string) (protocol.Klines, error) { return local.ReadMinute5(dir, code) }

// Deprecated: 使用 local.WriteDay。
func WriteDay(code string, ks protocol.Klines) ([]byte, error) { return local.WriteDay(code, ks) }

// Deprecated: 使用 local.WriteMinute1。
func WriteMinute1(code string, ks protocol.Klines) ([]byte, error) {
	return local.WriteMinute1(code, ks)
}

// Deprecated: 使用 local.WriteMinute5。
func WriteMinute5(code string, ks protocol.Klines) ([]byte, error) {
	return local.WriteMinute5(code, ks)
}

// Deprecated: 使用 export.KlinesToCSV。
func KlinesToCSV(filename, code, name string, ks protocol.Klines) error {
	return export.KlinesToCSV(filename, code, name, ks)
}

// Deprecated: 使用 export.TradesToCSV。
func TradesToCSV(filename string, ts protocol.Trades) error { return export.TradesToCSV(filename, ts) }

// Deprecated: 使用 pulltrade.PullTrade。
type PullTrade = pulltrade.PullTrade

// Deprecated: 使用 pulltrade.NewPullTrade。
func NewPullTrade(dir string) *PullTrade { return pulltrade.NewPullTrade(dir) }

// Deprecated: 使用 spider.Factor；该比例模型不同于 protocol.Factor。
type Factor = spider.Factor

// Deprecated: 使用 spider.Factors。
type Factors = spider.Factors

// Deprecated: 使用 spider 中对应的常量。
const (
	URLSinaFactorHFQ = spider.URLSinaFactorHFQ
	URLSinaFactorQFQ = spider.URLSinaFactorQFQ
	URLTHSDayKline   = spider.URLTHSDayKline
	SinaQFQ          = spider.SinaQFQ
	SinaHFQ          = spider.SinaHFQ
	THSBFQ           = spider.THSBFQ
	THSQFQ           = spider.THSQFQ
	THSHFQ           = spider.THSHFQ
)

// Deprecated: 使用 spider.GetSinaFactorFull。
func GetSinaFactorFull(code string) (Factors, error) { return spider.GetSinaFactorFull(code) }

// Deprecated: 使用 spider.GetSinaFactor。
func GetSinaFactor(code, adjustment string) (Factors, error) {
	return spider.GetSinaFactor(code, adjustment)
}

// Deprecated: 使用 spider.GetTHSDayKlineFactorFull。
func GetTHSDayKlineFactorFull(code string, c tdx.Client) ([3]protocol.Klines, []*Factor, error) {
	return spider.GetTHSDayKlineFactorFull(code, c)
}

// Deprecated: 使用 spider.GetTHSDayKlineFull。
func GetTHSDayKlineFull(code string, c tdx.Client) ([3]protocol.Klines, error) {
	return spider.GetTHSDayKlineFull(code, c)
}

// Deprecated: 使用 spider.GetTHSDayKline。
func GetTHSDayKline(code string, adjustment uint8) (protocol.Klines, error) {
	return spider.GetTHSDayKline(code, adjustment)
}

// Deprecated: 使用 httpserver.ListenCodesAndGbbqHTTP。
func ListenCodesAndGbbqHTTP(port int, codeOptions []codes.Option, equityOptions []gbbq.Option) error {
	return httpserver.ListenCodesAndGbbqHTTP(port, codeOptions, equityOptions)
}

// Deprecated: 使用 httpserver.ListenCodesHTTP。
func ListenCodesHTTP(port int, op ...codes.Option) error {
	return httpserver.ListenCodesHTTP(port, op...)
}

// Deprecated: 使用 metadatahttp.CodesHTTP。
type CodesHTTP = metadatahttp.CodesHTTP

// Deprecated: 使用 metadatahttp.GbbqHTTP。
type GbbqHTTP = metadatahttp.GbbqHTTP

// Deprecated: 使用 metadatahttp.DialCodesHTTP。
func DialCodesHTTP(address string, spec ...string) (*CodesHTTP, error) {
	return metadatahttp.DialCodesHTTP(address, spec...)
}

// Deprecated: 使用 metadatahttp.DialGbbqHTTP。
func DialGbbqHTTP(address string, spec ...string) (*GbbqHTTP, error) {
	return metadatahttp.DialGbbqHTTP(address, spec...)
}
