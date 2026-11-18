package market

import (
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// blockCodes 从板块文件(block_zs.dat)拉取板块指数代码列表。
// 板块指数代码如 880xxx/881xxx，归 sh 市场。
func (u *stdUnit) blockCodes(s *pull.Service) ([]model.Code, error) {
	pool, err := u.pool(s)
	if err != nil {
		return nil, err
	}
	var blocks []*protocol.Block
	err = pool.DoContext(s.Context(), func(c tdx.Client) error {
		c = tdx.WithContext(s.Context(), c)
		var err error
		blocks, err = c.GetBlockDataWithIndex(protocol.BlockFileZS)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := []model.Code{}
	for _, b := range blocks {
		if b.Index == "" {
			continue
		}
		// 板块指数代码(如 880xxx) 需带市场前缀，走 sh
		out = append(out, model.Code{
			Market: u.name,
			Code:   protocol.AddPrefix(b.Index),
			Name:   b.Name,
		})
	}
	return out, nil
}
