package market

import (
	"fmt"
	"path/filepath"
)

type instrumentMarketLoader interface {
	loadInstrumentMarkets(dir string) error
}

// LoadInstrumentMarkets loads and configures persistent market assignments.
// Call it at startup before importing a completed discovery result.
// 目录和归属表属于进程级共享数据，所有 Client 使用同一份；重复调用会替换它。
func LoadInstrumentMarkets(dir string) error {
	if dir == "" {
		return fmt.Errorf("证券市场归属目录不能为空")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	for prefix, market := range prefixMarkets {
		if loader, ok := market.(instrumentMarketLoader); ok {
			if err := loader.loadInstrumentMarkets(dir); err != nil {
				return fmt.Errorf("加载 %s 证券市场归属: %w", prefix, err)
			}
		}
	}
	return nil
}

// ImportInstruments gives each registered market a completed discovery result.
// 只有实现持久化归属的市场会接收数据，其余市场不参与。
func ImportInstruments(items []Instrument) error {
	for prefix, market := range prefixMarkets {
		importer, ok := market.(instrumentImporter)
		if !ok {
			continue
		}
		if err := importer.Import(items); err != nil {
			return fmt.Errorf("更新 %s 证券市场归属: %w", prefix, err)
		}
	}
	return nil
}
