package tdx

import "github.com/injoyai/tdx/v2/protocol"

func (c *MarketClient) GetBlockData(file string) ([]*protocol.Block, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetBlockData(file)
}

func (c *MarketClient) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetBlockDataWithIndex(file)
}

func (c *MarketClient) GetBlockFileRaw(file string) ([]byte, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetBlockFileRaw(file)
}

func (c *MarketClient) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	if _, err := standardInstrument(code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetCallAuction(code)
}

func (c *MarketClient) GetCompanyCategory(exchange protocol.Exchange, code string) ([]protocol.CompanyCategory, error) {
	if _, err := standardSecurity(exchange, code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetCompanyCategory(exchange, code)
}

func (c *MarketClient) GetCompanyContent(exchange protocol.Exchange, code, filename string, start, length uint32) (string, error) {
	if _, err := standardSecurity(exchange, code); err != nil {
		return "", err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return "", err
	}
	return link.GetCompanyContent(exchange, code, filename, start, length)
}

func (c *MarketClient) GetETFCodeAll() ([]string, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetETFCodeAll()
}

func (c *MarketClient) GetFinanceInfo(exchange protocol.Exchange, code string) (*protocol.FinanceInfo, error) {
	if _, err := standardSecurity(exchange, code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetFinanceInfo(exchange, code)
}

func (c *MarketClient) GetGbbq(code string) (*protocol.GbbqResp, error) {
	if _, err := standardInstrument(code); err != nil {
		return nil, err
	}
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetGbbq(code)
}

func (c *MarketClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetGbbqAll()
}

func (c *MarketClient) GetIndexCodeAll() ([]string, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetIndexCodeAll()
}

func (c *MarketClient) GetReportFile(file string) ([]byte, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetReportFile(file)
}

func (c *MarketClient) GetSpBlock() ([]*protocol.SpBlock, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetSpBlock()
}

func (c *MarketClient) GetStockCodeAll() ([]string, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetStockCodeAll()
}

func (c *MarketClient) GetTdxBk() ([]*protocol.TdxBk, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxBk()
}

func (c *MarketClient) GetTdxHy() ([]*protocol.TdxHy, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxHy()
}

func (c *MarketClient) GetTdxStat() ([]*protocol.TdxStat, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxStat()
}

func (c *MarketClient) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxStat2()
}

func (c *MarketClient) GetTdxZs() ([]*protocol.TdxZs, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetTdxZs()
}

func (c *MarketClient) GetXgsg() ([]*protocol.TdxXgsg, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetXgsg()
}

func (c *MarketClient) GetZHBFiles() (map[string][]byte, error) {
	link, err := c.connection(protocol.ExchangeSZ)
	if err != nil {
		return nil, err
	}
	return link.GetZHBFiles()
}
