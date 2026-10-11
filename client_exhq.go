package tdx

import (
	"context"
	"strings"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/module/tcp"
	"github.com/injoyai/tdx/v2/protocol"
)

// DialExHqDefault 连接默认扩展行情服务器(轮询 ExHosts)。
func DialExHqDefault(op ...client.Option) (*Client, error) {
	op = append([]client.Option{WithRedial()}, op...)
	return DialExHqHosts(ExHosts, op...)
}

// DialExHq 连接指定扩展行情服务器。
func DialExHq(addr string, op ...client.Option) (*Client, error) {
	if !strings.Contains(addr, ":") {
		addr += ":" + ExPort
	}
	return dialExHqWith(tcp.NewDial(addr), op...)
}

// DialExHqHosts 连接扩展行情服务器,多服务器轮询。
func DialExHqHosts(hosts []string, op ...client.Option) (*Client, error) {
	return dialExHqWith(NewExRangeDial(hosts), op...)
}

// dialExHqWith 建立扩展行情连接:握手用 ExSetup,心跳用品种数量请求。
func dialExHqWith(dial ios.DialFunc, op ...client.Option) (cli *Client, err error) {
	cli = &Client{
		Wait:        wait.New(time.Second * 10),
		waitTimeout: time.Second * 10,
		m:           maps.NewSafe(),
	}
	cli.Client, err = client.Dial(cli.cancelableDial(dial), func(c *client.Client) {
		c.Logger.Debug(true)
		c.Logger.SetLevel(LevelInfo)
		c.Logger.WithHEX()
		c.SetOption(op...)
		c.Event.OnReadFrom = protocol.ReadFrom
		c.Event.OnDealMessage = cli.handlerDealMessage
		c.Event.OnConnected = func(c *client.Client) error {
			w := cli.wrapWriter(c)
			<-cli.writeGate
			// 握手(响应忽略)
			if _, err := cli.Write(protocol.MEx.FrameSetup().Bytes()); err != nil {
				c.Close()
				return err
			}
			// 心跳:30s 发送品种数量请求(响应无等待者,被丢弃)
			cli.goHeartbeat(w, c.Closer.Done(), protocol.MEx.FrameCount().Bytes())
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	cli.cancelRun = cancel
	go cli.Client.Run(ctx)
	return cli, nil
}

// ---- 扩展行情 API ----

// ExMarkets 市场代码表。
func (this *Client) ExMarkets() ([]protocol.ExMarket, error) {
	r, err := this.SendFrame(protocol.MEx.FrameMarkets())
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMarket), nil
}

// ExCount 全部品种数量。
func (this *Client) ExCount() (int, error) {
	r, err := this.SendFrame(protocol.MEx.FrameCount())
	if err != nil {
		return 0, err
	}
	return r.(int), nil
}

// ExInstruments 品种(代码)列表,分页。
func (this *Client) ExInstruments(start uint32, count uint16) ([]protocol.ExInstrument, error) {
	r, err := this.SendFrame(protocol.MEx.FrameInstrument(start, count))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExInstrument), nil
}

// ExQuote 单品种五档行情。
func (this *Client) ExQuote(market uint8, code string) (*protocol.ExQuote, error) {
	r, err := this.SendFrame(protocol.MEx.FrameQuote(market, code))
	if err != nil {
		return nil, err
	}
	return r.(*protocol.ExQuote), nil
}

// ExQuoteList 批量行情列表(category 2=港股 3=期货)。
func (this *Client) ExQuoteList(market, category uint8, start, count uint16) ([]protocol.ExQuoteListItem, error) {
	r, err := this.SendFrame(protocol.MEx.FrameQuoteList(market, start, count), protocol.ExQuoteListCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExQuoteListItem), nil
}

// ExBars K线(category 同标准 KLINE 类型)。
func (this *Client) ExBars(category, market uint8, code string, start, count uint16) ([]protocol.ExKline, error) {
	r, err := this.SendFrame(protocol.MEx.FrameBars(category, market, code, start, count), protocol.ExBarsCache{Category: category})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExKline), nil
}

// ExMinute 当日分时。
func (this *Client) ExMinute(market uint8, code string) ([]protocol.ExMinuteTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameMinute(market, code))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// ExHistMinute 历史分时(date=YYYYMMDD)。
func (this *Client) ExHistMinute(market uint8, code string, date uint32) ([]protocol.ExMinuteTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameHistMinute(market, code, date))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExMinuteTick), nil
}

// ExTrade 当日分笔成交。
func (this *Client) ExTrade(market uint8, code string, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameTrade(market, code, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// ExHistTrade 历史分笔成交(date=YYYYMMDD)。
func (this *Client) ExHistTrade(market uint8, code string, date uint32, start, count uint16) ([]protocol.ExTradeTick, error) {
	r, err := this.SendFrame(protocol.MEx.FrameHistTrade(market, code, date, start, count), protocol.ExTradeCache{Market: market})
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExTradeTick), nil
}

// ExBarsRange 历史K线区间(date/date2=YYYYMMDD)。
func (this *Client) ExBarsRange(market uint8, code string, date, date2 uint32) ([]protocol.ExRangeKline, error) {
	r, err := this.SendFrame(protocol.MEx.FrameBarsRange(market, code, date, date2))
	if err != nil {
		return nil, err
	}
	return r.([]protocol.ExRangeKline), nil
}
