package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/injoyai/tdx/protocol"
)

var klineTypeMap = map[string]uint8{
	"1min":    protocol.TypeKlineMinute,
	"5min":    protocol.TypeKline5Minute,
	"15min":   protocol.TypeKline15Minute,
	"30min":   protocol.TypeKline30Minute,
	"60min":   protocol.TypeKline60Minute,
	"day":     protocol.TypeKlineDay,
	"week":    protocol.TypeKlineWeek,
	"month":   protocol.TypeKlineMonth,
	"quarter": protocol.TypeKlineQuarter,
	"year":    protocol.TypeKlineYear,
}

func main() {
	addr := flag.String("addr", ":8001", "HTTP listen address")
	poolSize := flag.Int("pool", 16, "TDX connection pool size")
	dataDir := flag.String("data", "./data", "data directory for SQLite databases")
	flag.Parse()

	pool, err := tdx.NewPool(dialClient, *poolSize)
	if err != nil {
		log.Fatalf("init pool: %v", err)
	}

	if err := initCodesCache(*dataDir); err != nil {
		log.Fatalf("init codes: %v", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, pool)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("TDX HTTP server listening on %s (pool=%d)", *addr, *poolSize)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
	pool.Close()
}

func initCodesCache(dataDir string) error {
	if tdx.DefaultCodes != nil {
		return nil
	}
	dbDir := filepath.Join(dataDir, "database")
	codes, err := tdx.NewCodes(
		tdx.WithCodesDialClient(dialClient),
		tdx.WithCodesDialDB(func() (*xorms.Engine, error) {
			return xorms.NewSqlite(filepath.Join(dbDir, "codes.db"))
		}),
	)
	if err != nil {
		return err
	}
	tdx.DefaultCodes = codes
	return nil
}

// --- routing ---

func registerRoutes(mux *http.ServeMux, pool *tdx.Pool) {
	reg := func(pattern string, fn func(*tdx.Pool, url.Values) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			data, err := fn(pool, r.URL.Query())
			if err != nil {
				var re reqErr
				if errors.As(err, &re) {
					writeErr(w, re.status, re.msg)
				} else {
					log.Printf("handler error: %v", err)
					writeErr(w, http.StatusInternalServerError, "internal error")
				}
				return
			}
			writeOK(w, data)
		})
	}

	reg("/api/ping", hPing)
	reg("/api/count", hCount)
	reg("/api/codes", hCodes)
	reg("/api/codes/all", hCodesAll)
	reg("/api/codes/stocks", hStockCodes)
	reg("/api/codes/etfs", hETFCodes)
	reg("/api/codes/indexes", hIndexCodes)
	reg("/api/quote", hQuote)
	reg("/api/minute", hMinute)
	reg("/api/minute/history", hMinuteHistory)
	reg("/api/trade", hTrade)
	reg("/api/trade/all", hTradeAll)
	reg("/api/trade/history", hTradeHistory)
	reg("/api/trade/history/day", hTradeHistoryDay)
	reg("/api/kline", hKline)
	reg("/api/kline/all", hKlineAll)
	reg("/api/index/kline", hIndexKline)
	reg("/api/index/kline/all", hIndexKlineAll)
	reg("/api/auction", hAuction)
	reg("/api/gbbq", hGbbq)
}

// --- handlers ---

func hPing(_ *tdx.Pool, _ url.Values) (any, error) {
	return "pong", nil
}

func hCount(pool *tdx.Pool, q url.Values) (any, error) {
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCount(ex)
		if err != nil {
			return nil, err
		}
		return map[string]uint16{"count": resp.Count}, nil
	})
}

func hCodes(pool *tdx.Pool, q url.Values) (any, error) {
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCode(ex, start)
		if err != nil {
			return nil, err
		}
		return toCodeResp(resp), nil
	})
}

func hCodesAll(pool *tdx.Pool, q url.Values) (any, error) {
	ex, err := parseExchange(q.Get("exchange"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCodeAll(ex)
		if err != nil {
			return nil, err
		}
		return toCodeResp(resp), nil
	})
}

func hStockCodes(pool *tdx.Pool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		codes, err := c.GetStockCodeAll()
		if err != nil {
			return nil, err
		}
		return toList(codes), nil
	})
}

func hETFCodes(pool *tdx.Pool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		codes, err := c.GetETFCodeAll()
		if err != nil {
			return nil, err
		}
		return toList(codes), nil
	})
}

func hIndexCodes(pool *tdx.Pool, _ url.Values) (any, error) {
	return doPool(pool, func(c *tdx.Client) (any, error) {
		codes, err := c.GetIndexCodeAll()
		if err != nil {
			return nil, err
		}
		return toList(codes), nil
	})
}

func hQuote(pool *tdx.Pool, q url.Values) (any, error) {
	codes, err := parseCodes(q.Get("codes"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetQuote(codes...)
		if err != nil {
			return nil, err
		}
		return toQuotes(resp), nil
	})
}

func hMinute(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetMinute(code)
		if err != nil {
			return nil, err
		}
		return toMinuteResp(resp, time.Now()), nil
	})
}

func hMinuteHistory(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	dateRaw, date, err := parseDate(q.Get("date"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetHistoryMinute(dateRaw, code)
		if err != nil {
			return nil, err
		}
		return toMinuteResp(resp, date), nil
	})
}

func hTrade(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 1800)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetMinuteTrade(code, start, count)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hTradeAll(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetMinuteTradeAll(code)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hTradeHistory(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	dateRaw, _, err := parseDate(q.Get("date"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 2000)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetHistoryMinuteTrade(dateRaw, code, start, count)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hTradeHistoryDay(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	dateRaw, _, err := parseDate(q.Get("date"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetHistoryMinuteTradeDay(dateRaw, code)
		if err != nil {
			return nil, err
		}
		return toTradeResp(resp), nil
	})
}

func hKline(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 800)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetKline(kt, code, start, count)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hKlineAll(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetKlineAll(kt, code)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hIndexKline(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	start, err := pUint16(q, "start", 0)
	if err != nil {
		return nil, err
	}
	count, err := pUint16(q, "count", 800)
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetIndex(kt, code, start, count)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hIndexKlineAll(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	kt, err := parseKlineType(q.Get("type"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetIndexAll(kt, code)
		if err != nil {
			return nil, err
		}
		return toKlineResp(resp), nil
	})
}

func hAuction(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetCallAuction(code)
		if err != nil {
			return nil, err
		}
		return toAuctionResp(resp), nil
	})
}

func hGbbq(pool *tdx.Pool, q url.Values) (any, error) {
	code, err := parseCode(q.Get("code"))
	if err != nil {
		return nil, err
	}
	return doPool(pool, func(c *tdx.Client) (any, error) {
		resp, err := c.GetGbbq(code)
		if err != nil {
			return nil, err
		}
		return toGbbqResp(resp), nil
	})
}

// --- pool helper ---

// dialClient creates a fresh TDX connection (same options as pool init).
var dialClient = func() (*tdx.Client, error) {
	return tdx.DialDefault(tdx.WithLevel(tdx.LevelError))
}

// replaceConn closes the old connection and dials a fresh one back into the pool.
// Retries up to 3 times with backoff to avoid permanent slot loss.
func replaceConn(pool *tdx.Pool, old *tdx.Client) {
	old.Close()
	for i := 0; i < 3; i++ {
		if nc, err := dialClient(); err == nil {
			pool.Put(nc)
			return
		} else {
			log.Printf("pool redial attempt %d: %v", i+1, err)
		}
		time.Sleep(time.Duration(i+1) * time.Second)
	}
	log.Printf("pool redial failed after 3 attempts, slot lost")
}

// doPool borrows a connection, runs fn, and returns the result.
// On connection error it retries once with a fresh connection before giving up.
func doPool(pool *tdx.Pool, fn func(*tdx.Client) (any, error)) (any, error) {
	c, err := pool.Get()
	if err != nil {
		return nil, err
	}
	result, err := fn(c)
	if err == nil {
		pool.Put(c)
		return result, nil
	}

	// First attempt failed — replace the bad connection and retry once.
	var re reqErr
	if errors.As(err, &re) {
		// Business-level error (bad params etc.), connection is fine.
		pool.Put(c)
		return nil, err
	}

	go replaceConn(pool, c)

	// Retry with a different connection.
	c2, err2 := pool.Get()
	if err2 != nil {
		return nil, err // return original error
	}
	result, err2 = fn(c2)
	if err2 != nil {
		go replaceConn(pool, c2)
		return nil, err2
	}
	pool.Put(c2)
	return result, nil
}

// --- param parsing ---

type reqErr struct {
	status int
	msg    string
}

func (e reqErr) Error() string { return e.msg }

func badReq(format string, args ...any) error {
	return reqErr{status: http.StatusBadRequest, msg: fmt.Sprintf(format, args...)}
}

func parseExchange(v string) (protocol.Exchange, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "sh":
		return protocol.ExchangeSH, nil
	case "sz":
		return protocol.ExchangeSZ, nil
	case "bj":
		return protocol.ExchangeBJ, nil
	case "":
		return 0, badReq("missing exchange")
	default:
		return 0, badReq("invalid exchange %q", v)
	}
}

func parseCode(v string) (string, error) {
	c := strings.TrimSpace(v)
	if c == "" {
		return "", badReq("missing code")
	}
	return c, nil
}

func parseCodes(v string) ([]string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, badReq("missing codes")
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		c := strings.TrimSpace(p)
		if c != "" {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return nil, badReq("missing codes")
	}
	return out, nil
}

func parseDate(v string) (string, time.Time, error) {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return "", time.Time{}, badReq("missing date")
	}
	t, err := time.Parse("20060102", raw)
	if err != nil {
		return "", time.Time{}, badReq("invalid date %q", raw)
	}
	return raw, t, nil
}

func parseKlineType(v string) (uint8, error) {
	k := strings.ToLower(strings.TrimSpace(v))
	if k == "" {
		return 0, badReq("missing type")
	}
	typ, ok := klineTypeMap[k]
	if !ok {
		return 0, badReq("invalid type %q", v)
	}
	return typ, nil
}

func pUint16(q url.Values, key string, def uint16) (uint16, error) {
	raw := strings.TrimSpace(q.Get(key))
	if raw == "" {
		return def, nil
	}
	n, err := strconv.ParseUint(raw, 10, 16)
	if err != nil {
		return 0, badReq("invalid %s %q", key, raw)
	}
	return uint16(n), nil
}

// --- JSON response ---

func writeOK(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "data": data})
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"code": 1, "msg": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("json encode: %v", err)
	}
}

// --- DTO converters ---

const timeFmt = "2006-01-02 15:04:05"

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(timeFmt)
}

func toList(ss []string) map[string]any {
	if ss == nil {
		ss = []string{}
	}
	return map[string]any{"list": ss}
}

func toCodeResp(r *protocol.CodeResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, c := range r.List {
		if c == nil {
			continue
		}
		list = append(list, map[string]any{
			"name":      c.Name,
			"code":      c.Code,
			"multiple":  c.Multiple,
			"decimal":   c.Decimal,
			"lastPrice": c.LastPrice,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toQuotes(resp protocol.QuotesResp) []map[string]any {
	out := make([]map[string]any, 0, len(resp))
	for _, q := range resp {
		if q == nil || q.Kline == nil {
			continue
		}
		k := q.Kline
		out = append(out, map[string]any{
			"exchange":   q.Exchange.String(),
			"code":       q.Code,
			"active1":    q.Active1,
			"totalHand":  k.Volume,
			"intuition":  q.Intuition,
			"amount":     k.Amount.Float64(),
			"insideDish": q.InsideDish,
			"outerDisc":  q.OuterDisc,
			"rate":       q.Rate,
			"active2":    q.Active2,
			"k": map[string]float64{
				"last":  k.Last.Float64(),
				"open":  k.Open.Float64(),
				"high":  k.High.Float64(),
				"low":   k.Low.Float64(),
				"close": k.Close.Float64(),
			},
			"buyLevel":  toPriceLevels(q.BuyLevel),
			"sellLevel": toPriceLevels(q.SellLevel),
		})
	}
	return out
}

func toPriceLevels(ls protocol.PriceLevels) []map[string]any {
	out := make([]map[string]any, len(ls))
	for i, l := range ls {
		out[i] = map[string]any{
			"buy":    l.Buy,
			"price":  l.Price.Float64(),
			"number": l.Number,
		}
	}
	return out
}

func toMinuteResp(r *protocol.MinuteResp, date time.Time) map[string]any {
	day := date.Format("2006-01-02")
	list := make([]map[string]any, 0, len(r.List))
	for _, p := range r.List {
		list = append(list, map[string]any{
			"time":   day + " " + p.Time + ":00",
			"price":  p.Price.Float64(),
			"number": p.Number,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toTradeResp(r *protocol.TradeResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, t := range r.List {
		if t == nil {
			continue
		}
		list = append(list, map[string]any{
			"time":   fmtTime(t.Time),
			"price":  t.Price.Float64(),
			"volume": t.Volume,
			"amount": t.Amount().Float64(),
			"status": t.Status,
			"number": t.Number,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toKlineResp(r *protocol.KlineResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, k := range r.List {
		if k == nil {
			continue
		}
		list = append(list, map[string]any{
			"time":      fmtTime(k.Time),
			"last":      k.Last.Float64(),
			"open":      k.Open.Float64(),
			"high":      k.High.Float64(),
			"low":       k.Low.Float64(),
			"close":     k.Close.Float64(),
			"order":     k.Order,
			"volume":    k.Volume,
			"amount":    k.Amount.Float64(),
			"upCount":   k.UpCount,
			"downCount": k.DownCount,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toAuctionResp(r *protocol.CallAuctionResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, a := range r.List {
		if a == nil {
			continue
		}
		list = append(list, map[string]any{
			"time":      fmtTime(a.Time),
			"price":     a.Price.Float64(),
			"match":     a.Match,
			"unmatched": a.Unmatched,
			"flag":      a.Flag,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}

func toGbbqResp(r *protocol.GbbqResp) map[string]any {
	list := make([]map[string]any, 0, len(r.List))
	for _, g := range r.List {
		if g == nil {
			continue
		}
		list = append(list, map[string]any{
			"code":     g.Code,
			"time":     fmtTime(g.Time),
			"category": g.Category,
			"c1":       g.C1,
			"c2":       g.C2,
			"c3":       g.C3,
			"c4":       g.C4,
		})
	}
	return map[string]any{"count": r.Count, "list": list}
}
