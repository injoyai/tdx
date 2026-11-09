package tdx

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/module/common"
)

const (
	LevelNone  = common.LevelNone
	LevelDebug = common.LevelDebug
	LevelWrite = common.LevelWrite
	LevelRead  = common.LevelRead
	LevelInfo  = common.LevelInfo
	LevelError = common.LevelError
	LevelAll   = common.LevelAll
)

// WithLogger 为连接设置日志；nil 使用 slog.Default，不修改应用的全局配置。
func WithLogger(logger *slog.Logger) client.Option {
	return func(c *client.Client) {
		if l, ok := c.Logger.(*connectionLogger); ok {
			l.mu.Lock()
			l.log = logger
			l.mu.Unlock()
		} else {
			c.Logger = newConnectionLogger(logger)
		}
	}
}

// WithDebug 控制通讯日志开关；响应解析错误仍通过连接的 slog.Logger 报告。
func WithDebug(b ...bool) client.Option {
	return func(c *client.Client) { c.Logger.Debug(b...) }
}

// WithLevel 设置通讯日志过滤；读写帧还须由 slog Handler 启用 Debug 级别。
func WithLevel(level int) client.Option {
	return func(c *client.Client) { c.Logger.SetLevel(level) }
}

// connectionLogger 适配 ios 的日志接口，保留通讯过滤与编码设置。
type connectionLogger struct {
	mu     sync.RWMutex
	log    *slog.Logger
	level  int
	debug  bool
	encode func([]byte) string
}

var _ common.Logger = (*connectionLogger)(nil)

func newConnectionLogger(logger *slog.Logger) *connectionLogger {
	return &connectionLogger{log: logger, level: LevelInfo, debug: true, encode: hex.EncodeToString}
}

func (l *connectionLogger) logger() *slog.Logger {
	l.mu.RLock()
	logger := l.log
	l.mu.RUnlock()
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func (cn *conn) logger() *slog.Logger {
	if cn.Client != nil {
		if l, ok := cn.Logger.(*connectionLogger); ok {
			return l.logger()
		}
	}
	return slog.Default()
}

func (l *connectionLogger) enabled(level int, severity slog.Level) (*slog.Logger, func([]byte) string, bool) {
	l.mu.RLock()
	enabled := l.debug && l.level != LevelNone && (l.level == LevelAll || level >= l.level)
	encode := l.encode
	l.mu.RUnlock()
	logger := l.logger()
	return logger, encode, enabled && logger.Enabled(context.Background(), severity)
}

func (l *connectionLogger) Readln(prefix string, p []byte) {
	if logger, encode, ok := l.enabled(LevelRead, slog.LevelDebug); ok {
		logger.Debug("接收数据", "prefix", prefix, "data", encode(p))
	}
}

func (l *connectionLogger) Writeln(prefix string, p []byte) {
	if logger, encode, ok := l.enabled(LevelWrite, slog.LevelDebug); ok {
		logger.Debug("发送数据", "prefix", prefix, "data", encode(p))
	}
}

func (l *connectionLogger) Infof(format string, v ...interface{}) {
	if logger, _, ok := l.enabled(LevelInfo, slog.LevelInfo); ok {
		logger.Info(strings.TrimRight(fmt.Sprintf(format, v...), "\r\n"))
	}
}

func (l *connectionLogger) Errorf(format string, v ...interface{}) {
	if logger, _, ok := l.enabled(LevelError, slog.LevelError); ok {
		logger.Error(strings.TrimRight(fmt.Sprintf(format, v...), "\r\n"))
	}
}

func (l *connectionLogger) SetEncode(encode func([]byte) string) {
	l.mu.Lock()
	l.encode = encode
	l.mu.Unlock()
}

func (l *connectionLogger) WithUTF8() { l.SetEncode(func(p []byte) string { return string(p) }) }
func (l *connectionLogger) WithHEX()  { l.SetEncode(hex.EncodeToString) }

func (l *connectionLogger) Debug(b ...bool) {
	l.mu.Lock()
	l.debug = len(b) == 0 || b[0]
	l.mu.Unlock()
}

func (l *connectionLogger) SetLevel(level int) {
	l.mu.Lock()
	l.level = level
	l.mu.Unlock()
}
