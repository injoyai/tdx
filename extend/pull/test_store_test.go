package pull

import (
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/internal/testutil"
)

type memoryStore = testutil.Pull[model.Code, model.KlineDay, model.KlineMinute, model.MinuteCoverage, model.MinuteEmptyScan]

func testStore(name string) *memoryStore {
	return &memoryStore{Namespace: name, Key: func(c model.Code) string { return c.Market.String() + "." + c.Code }, DayTime: func(k *model.KlineDay) string { return k.Time }, MinuteTime: func(k *model.KlineMinute) string { return k.Time }, Merge: func(old, c *model.MinuteCoverage) *model.MinuteCoverage {
		copy := *c
		if old != nil && old.TradeFallback == c.TradeFallback && (!c.TradeFallback || (old.Through >= c.From && c.Through >= old.From)) {
			copy.From = min(old.From, c.From)
			copy.Through = max(old.Through, c.Through)
		}
		return &copy
	}}
}

func hasMinutes(s *Service, c model.Code, y int) bool {
	at, err := s.LastMinuteTime(c, y)
	return err == nil && at != ""
}
