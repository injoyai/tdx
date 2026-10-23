package metadata

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol"
)

// CodeKey 将标准证券的合法输入形式统一为缓存使用的 sh/sz/bj 拼接代码。
func CodeKey(code string) (string, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return "", err
	}
	if i.Exchange > protocol.ExchangeBJ {
		return "", fmt.Errorf("元数据缓存不支持市场 %s", i.Exchange)
	}
	return i.Exchange.String() + i.Code, nil
}
