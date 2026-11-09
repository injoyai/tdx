package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/injoyai/tdx/v2"
)

func main() {
	if err := run(); err != nil {
		slog.Error("示例执行失败", "err", err)
		os.Exit(1)
	}
}

func run() error {
	flag.Parse()
	codes := flag.Args()
	if len(codes) == 0 {
		codes = []string{"sz000001", "sh600000", "sz159558"}
	}
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	resp, err := c.GetQuote(codes...)
	if err != nil {
		return err
	}
	for _, quote := range resp {
		fmt.Println(quote.Instrument, quote.Price, quote.Volume)
	}
	return nil
}
