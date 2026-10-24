package integration_test

import (
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type failingMetadataClient struct {
	tdx.Client
	err    error
	closes int
}

func (c *failingMetadataClient) Close() error { c.closes++; return nil }

func (c *failingMetadataClient) GetCodeAll(protocol.Exchange) (*tdx.CodeResp, error) {
	return nil, c.err
}

func (c *failingMetadataClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return nil, c.err
}

func (c *failingMetadataClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	return nil, c.err
}

func TestMetadataFailureOwnership(t *testing.T) {
	for _, kind := range []string{"codes", "gbbq", "workday"} {
		for _, ownClient := range []bool{false, true} {
			for _, ownDB := range []bool{false, true} {
				name := kind
				if ownClient {
					name += "/owned client"
				} else {
					name += "/shared client"
				}
				if ownDB {
					name += "/owned db"
				} else {
					name += "/shared db"
				}
				t.Run(name, func(t *testing.T) {
					db, err := xorms.NewSQLite(":memory:")
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					if err := db.Sync2(new(codes.CodeModel), new(protocol.Gbbq), new(workday.WorkdayModel)); err != nil {
						t.Fatal(err)
					}
					want := errors.New("initial update failed")
					c := &failingMetadataClient{err: want}
					var result any
					switch kind {
					case "codes":
						cop, dop := codes.WithClient(c), codes.WithDB(db)
						if ownClient {
							cop = codes.WithDialClient(func() (tdx.Client, error) { return c, nil })
						}
						if ownDB {
							dop = codes.WithDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						result, err = codes.New(cop, dop)
					case "gbbq":
						cop, dop := gbbq.WithClient(c), gbbq.WithDB(db)
						if ownClient {
							cop = gbbq.WithDialClient(func() (tdx.Client, error) { return c, nil })
						}
						if ownDB {
							dop = gbbq.WithDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						result, err = gbbq.New(cop, dop)
					case "workday":
						cop, dop := workday.WithClient(c), workday.WithDB(db)
						if ownClient {
							cop = workday.WithDialClient(func() (tdx.Client, error) { return c, nil })
						}
						if ownDB {
							dop = workday.WithDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						result, err = workday.New(cop, dop)
					}
					if !errors.Is(err, want) {
						t.Fatal("lost initialization error", err)
					}
					switch v := result.(type) {
					case *codes.Codes:
						if v != nil {
							t.Fatal("returned failed object")
						}
					case *gbbq.Gbbq:
						if v != nil {
							t.Fatal("returned failed object")
						}
					case *workday.Workday:
						if v != nil {
							t.Fatal("returned failed object")
						}
					}
					closed := 0
					if ownClient {
						closed = 1
					}
					if c.closes != closed {
						t.Fatalf("client closed %d times; want %d", c.closes, closed)
					}
					if err := db.Ping(); (err != nil) != ownDB {
						t.Fatal("database ownership violated", err)
					}
				})
			}
		}
	}
}

func TestMetadataDBOpenFailureClosesClient(t *testing.T) {
	want := errors.New("database open failed")
	for _, kind := range []string{"codes", "gbbq"} {
		t.Run(kind, func(t *testing.T) {
			c := &failingMetadataClient{}
			dial := func() (tdx.Client, error) { return c, nil }
			db := func() (*xorms.Engine, error) { return nil, want }
			var err error
			if kind == "codes" {
				_, err = codes.New(codes.WithDialClient(dial), codes.WithDialDB(db))
			} else {
				_, err = gbbq.New(gbbq.WithDialClient(dial), gbbq.WithDialDB(db))
			}
			if !errors.Is(err, want) || c.closes != 1 {
				t.Fatal("created client leaked", c.closes, err)
			}
		})
	}
}

func TestWorkdayClientOpenFailureClosesDB(t *testing.T) {
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := errors.New("client open failed")
	w, err := workday.New(workday.WithDialDB(func() (*xorms.Engine, error) { return db, nil }), workday.WithDialClient(func() (tdx.Client, error) { return nil, want }))
	if w != nil || !errors.Is(err, want) || db.Ping() == nil {
		t.Fatal("created db leaked", w, err)
	}
}
