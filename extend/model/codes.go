// Package model defines shared business data, without database or lifecycle dependencies.
package model

import (
	"fmt"
	"unicode/utf8"

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

type CodeModels []*CodeModel

// ValidateCodes 校验标准证券完整记录列表；空列表有效，拒绝空行、非规范代码和重复证券。
func ValidateCodes(rows CodeModels) error {
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if row == nil || utf8.RuneCountInString(row.Name) > 128 || utf8.RuneCountInString(row.Code) > 32 {
			return fmt.Errorf("model: invalid security record")
		}
		key, err := StandardCode(row.FullCode())
		if err != nil || len(key) < 2 || key[:2] != row.Exchange || key[2:] != row.Code || seen[key] {
			return fmt.Errorf("model: invalid or duplicate security %s", row.FullCode())
		}
		seen[key] = true
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
