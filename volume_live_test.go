package tdx

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// 使用原始帧，避免拿待验证的倍率解释来源数值；沿用全局串行限流。
func TestForeignVolumeSamplesLive(t *testing.T) {
	if os.Getenv("TDX_VOLUME_LIVE") != "1" {
		t.Skip("set TDX_VOLUME_LIVE=1 for rate-limited foreign samples")
	}
	c := liveTestClient(t, true, false)
	output := os.Getenv("TDX_VOLUME_OUTPUT")
	if output == "" {
		output = "output/volume-market-20261024"
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	samples := map[string]any{"captured_at": time.Now().Format(time.RFC3339)}
	defer func() {
		data, err := json.MarshalIndent(samples, "", "  ")
		if err == nil {
			err = os.WriteFile(output+"/foreign-raw.json", data, 0600)
		}
		if err != nil {
			t.Error(err)
		}
	}()
	codes := []string{"hk00700", "usAAPL", "hk.HSI", "hk.CES120"}
	if os.Getenv("TDX_VOLUME_VARIANTS") == "1" {
		codes = []string{"usAAPL", "hk.HSI", "hk.CES120"}
	}
	if configured := os.Getenv("TDX_VOLUME_CODES"); configured != "" {
		codes = strings.Split(configured, ",")
	}
	for _, code := range codes {
		i, info, err := instrumentInfo(code)
		if err != nil {
			t.Fatal(err)
		}
		requests := []struct {
			name  string
			frame *protocol.Frame
			cache any
		}{
			{"quote", protocol.MEx.FrameQuote(i.Exchange.Uint8(), i.Code), nil},
			{"day", protocol.MEx.FrameBars(info.Day.Category, i.Exchange.Uint8(), i.Code, 0, 2), protocol.ExBarsCache{Category: info.Day.Category}},
			{"minute", protocol.MEx.FrameBars(info.Minute.Category, i.Exchange.Uint8(), i.Code, 0, 2), protocol.ExBarsCache{Category: info.Minute.Category}},
			{"trade", protocol.MEx.FrameTrade(i.Exchange.Uint8(), i.Code, 0, 2), protocol.ExTradeCache{Market: i.Exchange.Uint8()}},
		}
		if os.Getenv("TDX_VOLUME_VARIANTS") == "1" {
			requests = nil
			for _, category := range []uint8{4, 9} {
				for _, count := range []uint16{1, 2} {
					requests = append(requests, struct {
						name  string
						frame *protocol.Frame
						cache any
					}{fmt.Sprintf("day-%d-count-%d", category, count), protocol.MEx.FrameBars(category, i.Exchange.Uint8(), i.Code, 0, count), protocol.ExBarsCache{Category: category}})
				}
			}
		}
		for _, request := range requests {
			value, err := c.SendFrame(request.frame, request.cache)
			if err != nil {
				t.Fatal(code, request.name, err)
			}
			samples[code+"/"+request.name] = value
		}
	}
}
