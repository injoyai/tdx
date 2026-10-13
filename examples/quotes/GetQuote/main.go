package main

import (
	"flag"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/examples/common"
)

func main() {
	flag.Parse()
	codes := flag.Args()
	if len(codes) == 0 {
		codes = []string{"sz000001", "sh600000", "sz159558"}
	}
	common.Run(func(c tdx.Client) {
		resp, err := c.GetQuote(codes...)
		logs.PanicErr(err)
		for _, quote := range resp {
			logs.Debug(quote.Instrument, quote.Price, quote.Volume, quote.VolumeUnit)
		}
	})
}
