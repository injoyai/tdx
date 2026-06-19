package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

func main() {
	code := "sz159915"
	if len(os.Args) > 1 {
		code = os.Args[1]
	}

	// 尝试多个节点
	hosts := []string{
		"119.97.185.59:7709",
		"111.230.186.52:7709",
		"122.51.120.217:7709",
		"124.71.9.153:7709",
	}

	var c *tdx.Client
	var err error
	for _, h := range hosts {
		c, err = tdx.Dial(h, tdx.WithRedial())
		if err == nil {
			time.Sleep(500 * time.Millisecond) // 等连接握手完成
			fmt.Printf("已连接: %s\n", h)
			break
		}
		fmt.Printf("连接失败 %s: %v\n", h, err)
	}
	if err != nil {
		log.Fatalf("所有节点均不可用")
	}
	defer c.Close()

	fmt.Printf("测试代码: %s (IsETF=%v)\n\n", code, protocol.IsETF(protocol.AddPrefix(code)))

	type testCase struct {
		name string
		fn   func() (uint16, error)
	}

	tests := []testCase{
		{"GetHistoryMinute", func() (uint16, error) {
			r, e := c.GetHistoryMinute("20260221", code)
			if e != nil {
				return 0, e
			}
			return r.Count, nil
		}},
		{"GetKlineDay", func() (uint16, error) {
			r, e := c.GetKlineDay(code, 0, 10)
			if e != nil {
				return 0, e
			}
			return r.Count, nil
		}},
		{"GetGbbq", func() (uint16, error) {
			r, e := c.GetGbbq(code)
			if e != nil {
				return 0, e
			}
			return r.Count, nil
		}},
		{"GetCallAuction", func() (uint16, error) {
			r, e := c.GetCallAuction(code)
			if e != nil {
				return 0, e
			}
			return r.Count, nil
		}},
		{"GetMinuteTrade", func() (uint16, error) {
			r, e := c.GetMinuteTrade(code, 0, 10)
			if e != nil {
				return 0, e
			}
			return r.Count, nil
		}},
	}

	pass, fail := 0, 0
	for _, t := range tests {
		cnt, err := t.fn()
		if err != nil {
			fmt.Printf("[FAIL] %-20s err=%v\n", t.name, err)
			fail++
		} else {
			fmt.Printf("[OK]   %-20s count=%d\n", t.name, cnt)
			pass++
		}
	}

	fmt.Printf("\n结果: %d pass, %d fail\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}
