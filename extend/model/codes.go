// Package model defines shared business data, without database or lifecycle dependencies.
package model

import (
	"fmt"
	"unicode/utf8"

	"github.com/injoyai/tdx/v2/protocol"
)

// StandardCode 将标准证券输入转为保留协议路由的规范代码。
func StandardCode(code string) (string, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return "", err
	}
	route, ok := i.Exchange.Route()
	if !ok || route.Protocol != protocol.ProtocolStandard {
		return "", fmt.Errorf("元数据缓存不支持市场 %s", i.Exchange)
	}
	return i.CodeKey(), nil
}

// CodeScope returns the CodeModel.Exchange key for a standard route.
// Single-route markets keep their existing abbreviation.
func CodeScope(ex protocol.Exchange) string {
	if route, ok := ex.Route(); ok && multipleRoutes(ex) {
		return route.ID.String()
	}
	return ex.String()
}

func multipleRoutes(ex protocol.Exchange) bool {
	route, ok := ex.Route()
	if !ok {
		return false
	}
	info, _ := route.ID.Info()
	return len(info.Routes) > 1
}

type CodeModel struct {
	Name      string  `json:"name"`      //名称,有时候名称会变,例STxxx
	Code      string  `json:"code"`      //代码
	Exchange  string  `json:"exchange"`  // 单路由简称，或多路由的 MarketID
	Multiple  uint16  `json:"multiple"`  //倍数
	Decimal   int8    `json:"decimal"`   //小数位
	LastPrice float64 `json:"lastPrice"` // 来源代码表的昨收参考值，保留其报价单位和有效性边界
}

// FullCode returns a query code retaining the row's route, e.g. sz000001.
func (c *CodeModel) FullCode() string {
	if ex, ok := protocol.MarketID(c.Exchange).Exchange(); ok && multipleRoutes(ex) {
		return (protocol.Instrument{Exchange: ex, Code: c.Code}).QueryCode()
	}
	return c.Exchange + c.Code
}

type CodeModels []*CodeModel

// ValidateCodes 校验标准证券完整记录列表；空列表有效，拒绝空行、非规范代码和重复证券。
func ValidateCodes(rows CodeModels) error {
	seen := make(map[protocol.Instrument]bool, len(rows))
	for _, row := range rows {
		if row == nil || utf8.RuneCountInString(row.Name) > 128 || utf8.RuneCountInString(row.Code) > 32 {
			return fmt.Errorf("model: invalid security record")
		}
		code := row.FullCode()
		i, err := protocol.ParseInstrument(code)
		route, known := i.Exchange.Route()
		if err != nil || !known || route.Protocol != protocol.ProtocolStandard ||
			row.Exchange != CodeScope(i.Exchange) || row.Code != i.Code || seen[i] {
			return fmt.Errorf("model: invalid or duplicate security %s", code)
		}
		seen[i] = true
	}
	return nil
}

func (cs CodeModels) Codes() []string {
	codes := make([]string, len(cs))
	for i, v := range cs {
		codes[i] = v.FullCode()
	}
	return codes
}
