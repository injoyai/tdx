package integration_test

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type metadataClient struct {
	tdx.Client
	closes atomic.Int32
	days   protocol.Klines
}

func (c *metadataClient) Close() error { c.closes.Add(1); return nil }

func (*metadataClient) GetCodeAll(protocol.Exchange) (*tdx.CodeResp, error) {
	return &tdx.CodeResp{}, nil
}

func (*metadataClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{}, nil
}

func (c *metadataClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	r := &tdx.KlineResp{Instrument: protocol.Instrument{Exchange: protocol.ExchangeSH, Code: "000001"}}
	for _, k := range c.days {
		r.List = append(r.List, tdx.Kline{Time: k.Time.Format(time.DateOnly), Standard: k})
	}
	r.Count = len(r.List)
	return r, nil
}

func TestMetadataCloseOwnership(t *testing.T) {
	for _, kind := range []string{"codes", "gbbq", "workday"} {
		for _, ownClient := range []bool{false, true} {
			for _, ownDB := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/client=%t/db=%t", kind, ownClient, ownDB), func(t *testing.T) {
					db, err := xorms.NewSQLite(":memory:")
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					c := &metadataClient{}
					var object interface {
						Update() error
						Close() error
					}
					switch kind {
					case "codes":
						cop, dop := codes.WithClient(c), codes.WithDB(db)
						if ownClient {
							cop = codes.WithDialClient(func() (tdx.Client, error) { return c, nil })
						}
						if ownDB {
							dop = codes.WithDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						object, err = codes.New(cop, dop, codes.WithSpec("@every 87600h"))
					case "gbbq":
						cop, dop := gbbq.WithClient(c), gbbq.WithDB(db)
						if ownClient {
							cop = gbbq.WithDialClient(func() (tdx.Client, error) { return c, nil })
						}
						if ownDB {
							dop = gbbq.WithDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						object, err = gbbq.New(cop, dop, gbbq.WithSpec("@every 87600h"))
					case "workday":
						cop, dop := workday.WithClient(c), workday.WithDB(db)
						if ownClient {
							cop = workday.WithDialClient(func() (tdx.Client, error) { return c, nil })
						}
						if ownDB {
							dop = workday.WithDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						object, err = workday.New(cop, dop, workday.WithSpec("@every 87600h"))
					}
					if err != nil {
						t.Fatal(err)
					}
					defer object.Close()
					var wg sync.WaitGroup
					for i := 0; i < 4; i++ {
						wg.Go(func() {
							if err := object.Close(); err != nil {
								t.Error(err)
							}
						})
					}
					wg.Wait()
					if err := object.Update(); !errors.Is(err, net.ErrClosed) {
						t.Fatal("updated after close", err)
					}
					want := int32(0)
					if ownClient {
						want = 1
					}
					if c.closes.Load() != want {
						t.Fatal("client ownership", c.closes.Load(), want)
					}
					if err := db.Ping(); (err != nil) != ownDB {
						t.Fatal("db ownership", err)
					}
				})
			}
		}
	}
}
