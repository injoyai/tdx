package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/injoyai/tdx/v2/protocol"
)

// Response 统一响应结构
type Response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

// respondOK 成功响应
func respondOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Response{Code: 0, Msg: "ok", Data: data})
}

// respondErr 错误响应
func respondErr(w http.ResponseWriter, httpCode int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpCode)
	_ = json.NewEncoder(w).Encode(Response{Code: 1, Msg: msg, Data: nil})
}

// ---- 参数解析辅助 ----

func queryStr(r *http.Request, key string) (string, error) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return "", fmt.Errorf("参数 %s 不能为空", key)
	}
	return v, nil
}

func queryUint8(r *http.Request, key string) (uint8, error) {
	s, err := queryStr(r, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(s, 10, 8)
	if err != nil {
		return 0, fmt.Errorf("参数 %s 格式错误: %s", key, s)
	}
	return uint8(n), nil
}

func queryUint16(r *http.Request, key string) (uint16, error) {
	s, err := queryStr(r, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("参数 %s 格式错误: %s", key, s)
	}
	return uint16(n), nil
}

func queryUint32(r *http.Request, key string) (uint32, error) {
	s, err := queryStr(r, key)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("参数 %s 格式错误: %s", key, s)
	}
	return uint32(n), nil
}

// queryUint16Default 解析 uint16,为空时返回默认值
func queryUint16Default(r *http.Request, key string, def uint16) uint16 {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return def
	}
	return uint16(n)
}

// parseExchange 将字符串转换为 protocol.Exchange
func parseExchange(s string) (protocol.Exchange, error) {
	switch s {
	case "sh", "SH":
		return protocol.ExchangeSH, nil
	case "sz", "SZ":
		return protocol.ExchangeSZ, nil
	case "bj", "BJ":
		return protocol.ExchangeBJ, nil
	default:
		return 0, fmt.Errorf("不支持的交易所: %s (可选: sh, sz, bj)", s)
	}
}
