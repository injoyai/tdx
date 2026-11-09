// Package httpclient 显式获取元数据 HTTP 快照，不启动定时任务、不持有查询缓存。
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
)

type Client struct {
	address string
	http    *http.Client
}

// New 不访问网络；client 为 nil 时使用 30 秒超时。注入的客户端由调用方管理。
func New(address string, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{address: strings.TrimRight(address, "/"), http: client}
}

func (c *Client) FetchCodes(ctx context.Context) (*metadata.Codes, error) {
	var records model.CodeModels
	if err := c.getJSON(ctx, "/codes", &records); err != nil {
		return nil, err
	}
	return metadata.NewCodes(records)
}

// FetchGbbq 完整校验分组、事件代码及日期后返回独立快照；失败不返回部分结果。
func (c *Client) FetchGbbq(ctx context.Context) (*metadata.Gbbq, error) {
	var groups map[string][]*model.Gbbq
	if err := c.getJSON(ctx, "/gbbqs", &groups); err != nil {
		return nil, err
	}
	var records []*model.Gbbq
	for code, rows := range groups {
		key, err := model.StandardCode(code)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row == nil {
				return nil, fmt.Errorf("httpclient: nil event for %s", code)
			}
			rowKey, err := model.StandardCode(row.Code)
			if err != nil {
				return nil, err
			}
			if rowKey != key {
				return nil, fmt.Errorf("httpclient: event code %q does not match %q", row.Code, code)
			}
		}
		records = append(records, rows...)
	}
	return metadata.NewGbbq(records)
}

func (c *Client) getJSON(ctx context.Context, path string, dest any) error {
	if ctx == nil {
		return fmt.Errorf("httpclient: nil context")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.address+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("httpclient: HTTP status %d", resp.StatusCode)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, dest)
}
