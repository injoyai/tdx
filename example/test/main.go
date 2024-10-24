package main

import (
	"bytes"
	"encoding/binary"
	"github.com/injoyai/goutil/g"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/server"
	"github.com/injoyai/ios/server/listen"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

func main() {

	c, err := tdx.Dial("124.71.187.122:7709")
	logs.PanicErr(err)

	_ = c

	/*
		发送：
		0c02000000011a001a003e05050000000000000002000030303030303101363030303038

		接收：
		b1cb74001c00000000000d005100bd00789c6378c1cecb252ace6066c5b4898987b9050ed1f90cc5b74c18a5bc18c1b43490fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf70148712002d7f1e13
		b1cb74000c02000000003e05ac00ac000102020000303030303031601294121a1c2d4eadabcf0ed412aae5fc01afb0024561124fbcc08301afa47900b2e3174100bf68871a4201b741b6144302bb09af334403972e96354504ac09b619560e00000000f8ff601201363030303038b60fba04060607429788a70efa04ada37ab2531c12974d91e7449dbc354184b6010001844bad324102b5679ea1014203a65abd8d0143048a6ba4dd01440587e101b3d2029613000000000000b60f
	*/
	resp, err := c.GetSecurityQuotes(map[protocol.Exchange]string{
		protocol.ExchangeSH: "000001",
		protocol.ExchangeSZ: "600008",
	})
	logs.PrintErr(err)

	for _, v := range resp {
		logs.Debugf("%#v\n", v)
	}

	select {}

}

// 监听第三方包发送的数据，确定协议用
func _listen() {
	listen.RunTCP(7709, func(s *server.Server) {
		s.SetClientOption(func(c *client.Client) {
			c.Logger.WithHEX()
		})
	})

	buf := new(bytes.Buffer)
	err := binary.Write(buf, binary.LittleEndian, g.Map{
		"name": "名称",
		"age":  17,
	})
	logs.PrintErr(err)
	logs.Debug(buf.String())
}
