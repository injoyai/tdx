package tdx

import (
	"encoding/hex"
	"github.com/injoyai/base/maps/wait/v2"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/client/dial"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/protocol"
	"time"
)

// Dial 与服务器建立连接
func Dial(addr string, op ...client.Option) (cli *Client, err error) {

	cli = &Client{
		w: wait.New(time.Second * 2),
	}

	cli.c, err = dial.TCP(addr, func(c *client.Client) {
		c.Logger.WithHEX() //以HEX显示
		c.SetOption(op...) //自定义选项
		//c.Event.OnReadFrom = protocol.ReadFrom     //分包
		c.Event.OnDealMessage = cli.handlerDealMessage //处理分包数据
	})
	if err != nil {
		return nil, err
	}

	go cli.c.Run()

	err = cli.connect()
	if err != nil {
		cli.c.Close()
		return nil, err
	}

	return cli, err
}

type Client struct {
	c     *client.Client
	w     *wait.Entity
	msgID uint32
}

// handlerDealMessage 处理服务器响应的数据
func (this *Client) handlerDealMessage(c *client.Client, msg ios.Acker) {

	f, err := protocol.Decode(msg.Payload())
	if err != nil {
		logs.Err(err)
		return
	}

	switch f.Type {
	case protocol.TypeSecurityQuote:
		resp := protocol.MSecurityQuote.Decode(f.Data)
		logs.Debug(resp)
		this.w.Done(conv.String(f.MsgID), resp)
		return

	}

	_ = f

}

func (this *Client) SendFrame(f *protocol.Frame) (any, error) {
	this.msgID++
	f.MsgID = this.msgID
	if _, err := this.c.Write(f.Bytes()); err != nil {
		return nil, err
	}
	return this.w.Wait(conv.String(this.msgID))
}

func (this *Client) Send(bs []byte) (any, error) {
	if _, err := this.c.Write(bs); err != nil {
		return nil, err
	}
	return this.w.Wait(conv.String(this.msgID))
}

func (this *Client) Write(bs []byte) (int, error) {
	return this.c.Write(bs)
}

func (this *Client) Close() error {
	return this.c.Close()
}

func (this *Client) connect() error {
	f := protocol.MConnect.Frame()
	_, err := this.Write(f.Bytes())
	return err
}

// GetSecurityList 获取市场内指定范围内的所有证券代码
// 0c02000000011a001a003e05050000000000000002000030303030303101363030303038
func (this *Client) GetSecurityList() (*protocol.SecurityListResp, error) {

	f := protocol.Frame{
		Control: 0x01,
		Type:    protocol.TypeConnect,
		Data:    nil,
	}
	_ = f

	bs, err := hex.DecodeString("0c02000000011a001a003e05050000000000000002000030303030303101363030303038")
	if err != nil {
		return nil, err
	}

	_, err = this.Write(bs)
	return nil, err

}

// GetSecurityQuotes 获取盘口五档报价
func (this *Client) GetSecurityQuotes(m map[protocol.Exchange]string) (protocol.SecurityQuotesResp, error) {
	f, err := protocol.MSecurityQuote.Frame(m)
	if err != nil {
		return nil, err
	}
	result, err := this.SendFrame(f)
	if err != nil {
		return nil, err
	}
	return result.(protocol.SecurityQuotesResp), nil
}
