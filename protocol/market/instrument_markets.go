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
func ImportInstruments(items []Instrument) error {
	for prefix, market := range prefixMarkets {
		if err := market.Import(items); err != nil {
			return fmt.Errorf("更新 %s 证券市场归属: %w", prefix, err)
		}
	}
	return nil
}
