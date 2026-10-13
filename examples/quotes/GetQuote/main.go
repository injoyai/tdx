package main

import (
	"flag"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
)

func main() {
	flag.Parse()
	codes := flag.Args()
	if len(codes) == 0 {
		codes = []string{"sz000001", "sh600000", "sz159558"}
	}
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()

	resp, err := c.GetQuote(codes...)
	logs.PanicErr(err)
	for _, quote := range resp {
		logs.Debug(quote.Instrument, quote.Price, quote.Volume, quote.VolumeUnit)
	}
}
