package sqlite

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

// Metadata provides concrete metadata persistence on a borrowed SQLite engine.
// Callers close the engine after finishing all I/O; this object starts no tasks.
type Metadata struct {
	db *xorm.Engine
	mu sync.Mutex
}

type codeRecord struct {
	ID        int64 `xorm:"pk autoincr"`
	Name      string
	Code      string `xorm:"unique(security)"`
	Exchange  string `xorm:"unique(security)"`
	Multiple  uint16
	Decimal   int8
	LastPrice float64
	EditDate  int64
	InDate    int64
}

func (*codeRecord) TableName() string { return "codes" }

type gbbqRecord protocol.Gbbq

func (*gbbqRecord) TableName() string { return "gbbq" }

type metadataState struct {
	Key       string `xorm:"pk"`
	FetchedAt int64
	From      string
	Through   string
	Source    string
}

type calendarDate struct {
	Market string `xorm:"pk"`
	Date   string `xorm:"pk"`
}

func NewMetadata(db *xorm.Engine) (*Metadata, error) {
	if db == nil {
		return nil, errors.New("sqlite: missing metadata engine")
	}
	for _, row := range []any{new(codeRecord), new(gbbqRecord), new(metadataState), new(calendarDate)} {
		if err := ensureTable(db, row); err != nil {
			return nil, err
		}
	}
	return &Metadata{db: db}, nil
}

func transactionContext(ctx context.Context, db *xorm.Engine, fn func(*xorm.Session) error) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	tx := db.NewSession().Context(ctx)
	defer tx.Close()
	if err := tx.Begin(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Metadata) column(name string) string { return s.db.GetColumnMapper().Obj2Table(name) }

func saveState(tx *xorm.Session, state metadataState) error {
	if _, err := tx.ID(state.Key).Delete(new(metadataState)); err != nil {
		return err
	}
	_, err := tx.Insert(&state)
	return err
}

// oldestRefresh returns zero when any requested scope has never been refreshed.
func oldestRefresh(tx *xorm.Session, keys []string, keyColumn string) (time.Time, error) {
	var oldest int64
	for start := 0; start < len(keys); start += 500 {
		batch := keys[start:min(start+500, len(keys))]
		var states []metadataState
		if err := tx.In(keyColumn, batch).Find(&states); err != nil {
			return time.Time{}, err
		}
		if len(states) != len(batch) {
			return time.Time{}, nil
		}
		for _, state := range states {
			if oldest == 0 || state.FetchedAt < oldest {
				oldest = state.FetchedAt
			}
		}
	}
	if oldest == 0 {
		return time.Time{}, nil
	}
	return time.Unix(0, oldest).UTC(), nil
}

func (s *Metadata) LoadCodes(ctx context.Context, exchanges []protocol.Exchange) (model.CodeSnapshot, error) {
	out := model.CodeSnapshot{Exchanges: append([]protocol.Exchange(nil), exchanges...)}
	if err := (model.CodeSnapshot{Exchanges: exchanges, FetchedAt: time.Now()}).Validate(); err != nil {
		return out, err
	}
	var scope []string
	var keys []string
	for _, ex := range exchanges {
		scope = append(scope, ex.String())
		keys = append(keys, "codes:"+ex.String())
	}
	err := transactionContext(ctx, s.db, func(tx *xorm.Session) error {
		if err := tx.Table("codes").In(s.column("Exchange"), scope).Asc(s.column("Exchange"), s.column("Code")).Find(&out.Rows); err != nil {
			return err
		}
		var err error
		out.FetchedAt, err = oldestRefresh(tx, keys, s.column("Key"))
		return err
	})
	return out, err
}

func (s *Metadata) ReplaceCodes(ctx context.Context, snapshot model.CodeSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return transactionContext(ctx, s.db, func(tx *xorm.Session) error {
		var scope []string
		for _, ex := range snapshot.Exchanges {
			scope = append(scope, ex.String())
		}
		var old []*codeRecord
		if err := tx.In(s.column("Exchange"), scope).Find(&old); err != nil {
			return err
		}
		existing := map[string]*codeRecord{}
		fresh := map[string]bool{}
		for _, row := range snapshot.Rows {
			fresh[row.FullCode()] = true
		}
		for _, row := range old {
			existing[row.Exchange+row.Code] = row
			if !fresh[row.Exchange+row.Code] {
				if _, err := tx.ID(row.ID).Delete(new(codeRecord)); err != nil {
					return err
				}
			}
		}
		var inserts []*codeRecord
		now := snapshot.FetchedAt.Unix()
		for _, row := range snapshot.Rows {
			v := &codeRecord{Name: row.Name, Code: row.Code, Exchange: row.Exchange, Multiple: row.Multiple, Decimal: row.Decimal, LastPrice: row.LastPrice}
			prior := existing[row.FullCode()]
			if prior == nil {
				v.InDate, v.EditDate = now, now
				inserts = append(inserts, v)
			} else if prior.Name != row.Name || prior.Multiple != row.Multiple || prior.Decimal != row.Decimal || prior.LastPrice != row.LastPrice {
				v.EditDate = now
				fields := []string{"Name", "Multiple", "Decimal", "LastPrice", "EditDate"}
				for i, field := range fields {
					fields[i] = s.column(field)
				}
				if _, err := tx.ID(prior.ID).Cols(fields...).Update(v); err != nil {
					return err
				}
			}
		}
		for i := 0; i < len(inserts); i += 100 {
			if _, err := tx.Insert(inserts[i:min(i+100, len(inserts))]); err != nil {
				return err
			}
		}
		for _, ex := range snapshot.Exchanges {
			if err := saveState(tx, metadataState{Key: "codes:" + ex.String(), FetchedAt: snapshot.FetchedAt.UnixNano()}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Metadata) LoadGbbq(ctx context.Context, codes []string) (model.GbbqSnapshot, error) {
	out := model.GbbqSnapshot{Codes: append([]string(nil), codes...)}
	if err := (model.GbbqSnapshot{Codes: codes, FetchedAt: time.Now()}).Validate(); err != nil {
		return out, err
	}
	err := transactionContext(ctx, s.db, func(tx *xorm.Session) error {
		// Limit IN lists as well as insert batches for older SQLite builds.
		var keys []string
		for start := 0; start < len(codes); start += 500 {
			var rows []*model.Gbbq
			if err := tx.Table("gbbq").In(s.column("Code"), codes[start:min(start+500, len(codes))]).Asc(s.column("Time")).Find(&rows); err != nil {
				return err
			}
			for _, row := range rows {
				row.Time = row.Time.In(protocol.Location())
			}
			out.Rows = append(out.Rows, rows...)
		}
		for _, code := range codes {
			keys = append(keys, "gbbq:"+code)
		}
		var err error
		out.FetchedAt, err = oldestRefresh(tx, keys, s.column("Key"))
		return err
	})
	sort.SliceStable(out.Rows, func(i, j int) bool {
		if out.Rows[i].Code != out.Rows[j].Code {
			return out.Rows[i].Code < out.Rows[j].Code
		}
		return out.Rows[i].Time.Before(out.Rows[j].Time)
	})
	return out, err
}

func (s *Metadata) ReplaceGbbq(ctx context.Context, snapshot model.GbbqSnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return transactionContext(ctx, s.db, func(tx *xorm.Session) error {
		for start := 0; start < len(snapshot.Codes); start += 500 {
			if _, err := tx.In(s.column("Code"), snapshot.Codes[start:min(start+500, len(snapshot.Codes))]).Delete(new(gbbqRecord)); err != nil {
				return err
			}
		}
		for start := 0; start < len(snapshot.Rows); start += 100 {
			var batch []*gbbqRecord
			for _, row := range snapshot.Rows[start:min(start+100, len(snapshot.Rows))] {
				copy := gbbqRecord(*row)
				batch = append(batch, &copy)
			}
			if _, err := tx.Insert(batch); err != nil {
				return err
			}
		}
		for start := 0; start < len(snapshot.Codes); start += 100 {
			var keys []string
			var states []*metadataState
			for _, code := range snapshot.Codes[start:min(start+100, len(snapshot.Codes))] {
				key := "gbbq:" + code
				keys = append(keys, key)
				states = append(states, &metadataState{Key: key, FetchedAt: snapshot.FetchedAt.UnixNano()})
			}
			if _, err := tx.In(s.column("Key"), keys).Delete(new(metadataState)); err != nil {
				return err
			}
			if _, err := tx.Insert(states); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Metadata) LoadWorkdays(ctx context.Context, market protocol.Market) (model.WorkdaySnapshot, error) {
	out := model.WorkdaySnapshot{Market: market}
	err := transactionContext(ctx, s.db, func(tx *xorm.Session) error {
		var state metadataState
		has, err := tx.ID("workday:" + market.String()).Get(&state)
		if err != nil || !has {
			return err
		}
		out.From, out.Through, out.Source = state.From, state.Through, state.Source
		out.FetchedAt = time.Unix(0, state.FetchedAt).UTC()
		var rows []calendarDate
		if err := tx.Where("`"+s.column("Market")+"`=?", market.String()).Asc(s.column("Date")).Find(&rows); err != nil {
			return err
		}
		for _, row := range rows {
			out.Dates = append(out.Dates, row.Date)
		}
		return nil
	})
	return out, err
}

func (s *Metadata) SaveWorkdays(ctx context.Context, snapshot model.WorkdaySnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if snapshot.From == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("sqlite: refusing empty calendar coverage")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return transactionContext(ctx, s.db, func(tx *xorm.Session) error {
		state := metadataState{Key: "workday:" + snapshot.Market.String()}
		has, err := tx.ID(state.Key).Get(&state)
		if err != nil {
			return err
		}
		from, through := snapshot.From, snapshot.Through
		if has {
			oldEnd, _ := time.Parse(time.DateOnly, state.Through)
			newEnd, _ := time.Parse(time.DateOnly, through)
			if state.Source != snapshot.Source || from > oldEnd.AddDate(0, 0, 1).Format(time.DateOnly) || state.From > newEnd.AddDate(0, 0, 1).Format(time.DateOnly) {
				return fmt.Errorf("sqlite: incompatible or disjoint calendar coverage")
			}
			from, through = min(from, state.From), max(through, state.Through)
		}
		// Replace only the scanned range: corrections can remove previously observed dates.
		if _, err := tx.Where("`"+s.column("Market")+"`=? AND `"+s.column("Date")+"`>=? AND `"+s.column("Date")+"`<=?", snapshot.Market.String(), snapshot.From, snapshot.Through).Delete(new(calendarDate)); err != nil {
			return err
		}
		for start := 0; start < len(snapshot.Dates); start += 100 {
			var rows []*calendarDate
			for _, date := range snapshot.Dates[start:min(start+100, len(snapshot.Dates))] {
				rows = append(rows, &calendarDate{Market: snapshot.Market.String(), Date: date})
			}
			if _, err := tx.Insert(rows); err != nil {
				return err
			}
		}
		return saveState(tx, metadataState{Key: state.Key, From: from, Through: through, Source: snapshot.Source, FetchedAt: snapshot.FetchedAt.UnixNano()})
	})
}
