package tdx

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

func readLogRecords(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(data))
	var records []map[string]any
	for decoder.More() {
		var record map[string]any
		if err := decoder.Decode(&record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func TestConnectionLoggerLevels(t *testing.T) {
	for _, tc := range []struct {
		name  string
		level int
		want  []string
	}{
		{"none", LevelNone, nil},
		{"all", LevelAll, []string{"DEBUG", "DEBUG", "INFO", "ERROR"}},
		{"debug", LevelDebug, []string{"DEBUG", "DEBUG", "INFO", "ERROR"}},
		{"write", LevelWrite, []string{"DEBUG", "DEBUG", "INFO", "ERROR"}},
		{"read", LevelRead, []string{"DEBUG", "INFO", "ERROR"}},
		{"info", LevelInfo, []string{"INFO", "ERROR"}},
		{"error", LevelError, []string{"ERROR"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := newConnectionLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			logger.SetLevel(tc.level)
			logger.Writeln("peer", []byte{0, 16, 255})
			logger.Readln("peer", []byte{0, 16, 255})
			logger.Infof("connected %d\n", 1)
			logger.Errorf("failed %d\n", 2)
			var levels []string
			for _, record := range readLogRecords(t, buf.Bytes()) {
				levels = append(levels, record["level"].(string))
				if record["level"] == "DEBUG" && (record["data"] != "0010ff" || record["prefix"] != "peer") {
					t.Fatal("HEX encoding or peer prefix changed", record)
				}
			}
			if !reflect.DeepEqual(levels, tc.want) {
				t.Fatalf("levels=%v, want %v", levels, tc.want)
			}
		})
	}
}

func TestWithLoggerPreservesCommunicationOptions(t *testing.T) {
	for _, loggerFirst := range []bool{false, true} {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		c := &client.Client{Logger: newConnectionLogger(nil)}
		originalDefault := slog.Default()
		if loggerFirst {
			WithLogger(logger)(c)
		}
		WithLevel(LevelRead)(c)
		WithDebug(false)(c)
		c.Logger.WithUTF8()
		if !loggerFirst {
			WithLogger(logger)(c)
		}
		c.Logger.Readln("peer", []byte("hidden"))
		c.Logger.Infof("hidden")
		c.Logger.Errorf("hidden")
		if buf.Len() != 0 {
			t.Fatal("disabled communication logs were emitted")
		}
		WithDebug()(c)
		c.Logger.Writeln("peer", []byte("filtered"))
		c.Logger.Readln("peer", []byte("文字"))
		records := readLogRecords(t, buf.Bytes())
		if len(records) != 1 || records[0]["data"] != "文字" {
			t.Fatal("option order or UTF8 encoding changed", records)
		}
		if (&conn{Client: c}).logger() != logger || slog.Default() != originalDefault {
			t.Fatal("connection logger was not isolated from the global logger")
		}
		WithLogger(nil)(c)
		if (&conn{Client: c}).logger() != slog.Default() {
			t.Fatal("nil logger did not use slog.Default")
		}
	}
}

func TestConnectionLoggerSkipsDisabledEncoding(t *testing.T) {
	var encoded atomic.Int32
	logger := newConnectionLogger(slog.New(slog.NewJSONHandler(io.Discard, nil)))
	logger.SetLevel(LevelAll)
	logger.SetEncode(func(p []byte) string { encoded.Add(1); return string(p) })
	logger.Readln("peer", []byte("hidden at slog Info level"))
	if encoded.Load() != 0 {
		t.Fatal("disabled Debug record encoded its payload")
	}
}

func TestConnectionLoggerConcurrentOptions(t *testing.T) {
	a := slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	b := a.With("connection", "test")
	logger := newConnectionLogger(a)
	c := &client.Client{Logger: logger}
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := 0; i < 200; i++ {
			logger.Writeln("peer", []byte("payload"))
			logger.Readln("peer", []byte("payload"))
			logger.Infof("info %d", i)
			logger.Errorf("error %d", i)
		}
	})
	wg.Go(func() {
		for i := 0; i < 200; i++ {
			logger.Debug(i%2 == 0)
			logger.SetLevel(LevelNone)
			logger.WithHEX()
			WithLogger(b)(c)
			logger.SetLevel(LevelAll)
			logger.WithUTF8()
			WithLogger(a)(c)
		}
	})
	wg.Wait()
}

func TestDecodeErrorsUseConnectionLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	c := pipeTestClient(t, false, func(s net.Conn) {
		if _, _, err := readTestFrame(s); err != nil {
			return
		}
		h, _, err := readTestFrame(s)
		if err != nil {
			return
		}
		if err := writeTestResponse(s, h, []byte{1}); err != nil {
			return
		}
		io.Copy(io.Discard, s)
	}, WithLogger(logger))
	if _, err := c.GetCount(protocol.ExchangeSZ); err == nil {
		t.Fatal("malformed count response accepted")
	}
	records := readLogRecords(t, buf.Bytes())
	if len(records) != 1 || records[0]["msg"] != "解码行情响应失败" || records[0]["level"] != "ERROR" || records[0]["err"] == nil {
		t.Fatal("decode error did not reach the injected logger", records)
	}
}
