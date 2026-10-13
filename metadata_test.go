package tdx

import (
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type failingMetadataClient struct {
	Client
	err    error
	closes int
}

func (c *failingMetadataClient) Close() error                                     { c.closes++; return nil }
func (c *failingMetadataClient) GetCodeAll(protocol.Exchange) (*CodeResp, error)  { return nil, c.err }
func (c *failingMetadataClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) { return nil, c.err }
func (c *failingMetadataClient) GetKlineDayAll(string) (*KlineResp, error)        { return nil, c.err }

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
					db, err := xorms.NewSqlite(":memory:")
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					if err := db.Sync2(new(CodeModel), new(protocol.Gbbq), new(WorkdayModel)); err != nil {
						t.Fatal(err)
					}
					want := errors.New("initial update failed")
					c := &failingMetadataClient{err: want}
					var result any
					switch kind {
					case "codes":
						cop, dop := WithCodesClient(c), WithCodesDB(db)
						if ownClient {
							cop = WithCodesDialClient(func() (Client, error) { return c, nil })
						}
						if ownDB {
							dop = WithCodesDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						result, err = NewCodes(cop, dop)
					case "gbbq":
						cop, dop := WithGbbqClient(c), WithGbbqDB(db)
						if ownClient {
							cop = WithGbbqDialClient(func() (Client, error) { return c, nil })
						}
						if ownDB {
							dop = WithGbbqDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						result, err = NewGbbq(cop, dop)
					case "workday":
						cop, dop := WithWorkdayClient(c), WithWorkdayDB(db)
						if ownClient {
							cop = WithWorkdayDialClient(func() (Client, error) { return c, nil })
						}
						if ownDB {
							dop = WithWorkdayDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						result, err = NewWorkday(cop, dop)
					}
					if !errors.Is(err, want) {
						t.Fatal("lost initialization error", err)
					}
					switch v := result.(type) {
					case *Codes:
						if v != nil {
							t.Fatal("returned failed object")
						}
					case *Gbbq:
						if v != nil {
							t.Fatal("returned failed object")
						}
					case *Workday:
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
			dial := func() (Client, error) { return c, nil }
			db := func() (*xorms.Engine, error) { return nil, want }
			var err error
			if kind == "codes" {
				_, err = NewCodes(WithCodesDialClient(dial), WithCodesDialDB(db))
			} else {
				_, err = NewGbbq(WithGbbqDialClient(dial), WithGbbqDialDB(db))
			}
			if !errors.Is(err, want) || c.closes != 1 {
				t.Fatal("created client leaked", c.closes, err)
			}
		})
	}
}

func TestWorkdayClientOpenFailureClosesDB(t *testing.T) {
	db, err := xorms.NewSqlite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := errors.New("client open failed")
	w, err := NewWorkday(WithWorkdayDialDB(func() (*xorms.Engine, error) { return db, nil }), WithWorkdayDialClient(func() (Client, error) { return nil, want }))
	if w != nil || !errors.Is(err, want) || db.Ping() == nil {
		t.Fatal("created db leaked", w, err)
	}
}
