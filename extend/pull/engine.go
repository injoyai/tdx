package pull

import (
	"errors"
	"sync"

	"github.com/injoyai/tdx/lib/xorms"
)

type cachedEngine struct {
	db   *xorms.Engine
	refs int
	used uint64
}

// engine 借用引擎；调用者必须在查询/事务结束后 release，使用中不会被淘汰。
func (s *Service) engine(filename string, table any) (*xorms.Engine, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, errors.New("pull: service 已关闭")
	}
	entry := s.engines[filename]
	if entry == nil {
		db, err := openDB(filename, table)
		if err != nil {
			return nil, nil, err
		}
		entry = &cachedEngine{db: db}
		s.engines[filename] = entry
	}
	entry.refs++
	s.engineClock++
	entry.used = s.engineClock
	s.evictEngines()
	var once sync.Once
	release := func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			entry.refs--
			if s.closed && entry.refs == 0 {
				_ = entry.db.Close()
				delete(s.engines, filename)
			} else {
				s.evictEngines()
			}
		})
	}
	return entry.db, release, nil
}

// evictEngines 在持锁时淘汰最久未使用的空闲引擎。并发借用可以短暂超限。
func (s *Service) evictEngines() {
	for len(s.engines) > s.engineLimit {
		var oldest *cachedEngine
		var path string
		for name, entry := range s.engines {
			if entry.refs == 0 && (oldest == nil || entry.used < oldest.used) {
				oldest, path = entry, name
			}
		}
		if oldest == nil {
			return
		}
		_ = oldest.db.Close()
		delete(s.engines, path)
	}
}
