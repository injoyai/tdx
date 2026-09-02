// Package market 实现各市场的拉取 Unit（可插拔）。
// 板块市场：拉取板块指数(880xxx/881xxx)的日线/分钟线。
package market

import (
	"context"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// Block 板块市场：板块指数(880xxx/881xxx) 走标准行情 GetIndex 拉取。
// 成分代码（板块内个股）不在此拉取，通过 Config.Codes["block"] 可自由指定。
type Block struct{ stdUnit }

var _ pull.Unit = (*Block)(nil)

func init() {
	pull.Register(&Block{stdUnit{name: "block", kind: "index"}})
}

// Codes 从板块文件(block_zs.dat)拉取板块指数代码列表。
// 板块指数代码如 880xxx/881xxx，归 sh 市场。
func (u *Block) Codes(ctx context.Context, s *pull.Service) ([]pull.Code, error) {
	m, err := u.Manage(s)
	if err != nil {
		return nil, err
	}
	var blocks []*protocol.Block
	err = m.Do(func(c *tdx.Client) error {
		var err error
		blocks, err = c.GetBlockDataWithIndex(protocol.BlockFileZS)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, b := range blocks {
		if b.Index == "" {
			continue
		}
		// 板块指数代码(如 880xxx) 需带市场前缀，走 sh
		out = append(out, pull.Code{
			Market: u.name,
			Code:   protocol.AddPrefix(b.Index),
			Name:   b.Name,
		})
	}
	return out, nil
}

// FetchDay 板块指数日线：走 stdUnit 的 index 逻辑。
func (u *Block) FetchDay(ctx context.Context, s *pull.Service, code pull.Code) error {
	return u.stdUnit.FetchDay(ctx, s, code)
}

// FetchMin 板块指数分钟线：走 stdUnit 的 index 逻辑。
func (u *Block) FetchMin(ctx context.Context, s *pull.Service, code pull.Code) error {
	return u.stdUnit.FetchMin(ctx, s, code)
}
