// Package model defines shared business data, without database or lifecycle dependencies.
package model

import (
	"fmt"
	"math"

	"github.com/injoyai/tdx/v2/protocol"
)

// StandardCode 将标准证券的合法输入形式统一为 sh/sz/bj 拼接代码。
func StandardCode(code string) (string, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return "", err
	}
	if i.Exchange > protocol.ExchangeBJ {
		return "", fmt.Errorf("元数据缓存不支持市场 %s", i.Exchange)
	}
	return i.Exchange.String() + i.Code, nil
}

type CodeModel struct {
	Name      string  `json:"name"`      //名称,有时候名称会变,例STxxx
	Code      string  `json:"code"`      //代码
	Exchange  string  `json:"exchange"`  //交易所
	Multiple  uint16  `json:"multiple"`  //倍数
	Decimal   int8    `json:"decimal"`   //小数位
	LastPrice float64 `json:"lastPrice"` // 来源代码表的昨收参考值，保留其报价单位和有效性边界
}

// FullCode 获取完整代码 sz000001
func (c *CodeModel) FullCode() string {
	return c.Exchange + c.Code
}

// Price 按代码精度校准标准报价；p 已按默认整数分换算，返回实际元/指数点报价。
func (c *CodeModel) Price(p float64) float64 {
	return p * math.Pow10(int(2-c.Decimal))
}

type CodeModels []*CodeModel

func (cs CodeModels) Codes() []string {
	codes := make([]string, len(cs))
	for i, v := range cs {
		codes[i] = v.FullCode()
	}
	return codes
}
