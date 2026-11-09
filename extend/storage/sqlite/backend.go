package sqlite

import (
	"errors"
	"path/filepath"
	"sync"

	"xorm.io/xorm"
)

// Store combines BarStore with concrete metadata persistence methods.
type Store struct {
	*PullStore
	*Metadata
	metadataDB *xorm.Engine
	once       sync.Once
	closeErr   error
}

// New opens a dataset. Construction performs local schema initialization only.
// The caller closes the backend after all users have stopped.
func New(dir string) (*Store, error) {
	bars, err := NewPull(dir, 32)
	if err != nil {
		return nil, err
	}
	db, err := Open(filepath.Join(bars.dir, "metadata.db"))
	if err != nil {
		return nil, errors.Join(err, bars.Close())
	}
	metadata, err := NewMetadata(db)
	if err != nil {
		return nil, errors.Join(err, db.Close(), bars.Close())
	}
	return &Store{PullStore: bars, Metadata: metadata, metadataDB: db}, nil
}

func (s *Store) Close() error {
	s.once.Do(func() { s.closeErr = errors.Join(s.PullStore.Close(), s.metadataDB.Close()) })
	return s.closeErr
}
