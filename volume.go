package tdx

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol"
)

func checkSupport(i protocol.Instrument, operation string, supported bool) error {
	if !supported {
		return fmt.Errorf("%s 的统一行情接口不支持%s", i, operation)
	}
	return nil
}

// volumeScale validates quote support before reading its quantity conversion.
func volumeScale(i protocol.Instrument) (int64, error) {
	info, ok := i.Rules()
	if !ok {
		return 0, fmt.Errorf("%s 的市场未定义行情规则", i)
	}
	if err := checkSupport(i, "报价", info.Quote.Supported); err != nil {
		return 0, err
	}
	if !info.Quote.Fields.Volume {
		return 0, nil
	}
	if info.Quote.VolumeScale <= 0 {
		return 0, fmt.Errorf("%s 的报价成交量倍率无效", i)
	}
	return info.Quote.VolumeScale, nil
}

// klineVolumeScale validates the declared query support and volume field.
func klineVolumeScale(i protocol.Instrument, rule protocol.KlineRule) (int64, error) {
	if err := checkSupport(i, fmt.Sprintf("K线周期 %d", rule.Category), rule.Supported); err != nil {
		return 0, err
	}
	if rule.Fields.Amount && (rule.AmountField == protocol.NoKlineAmount || rule.AmountScale <= 0) {
		return 0, fmt.Errorf("%s 周期 %d 的成交额规则无效", i, rule.Category)
	}
	if !rule.Fields.Volume {
		return 0, nil
	}
	if rule.VolumeScale <= 0 {
		return 0, fmt.Errorf("%s 周期 %d 的成交量倍率无效", i, rule.Category)
	}
	return rule.VolumeScale, nil
}

func scaleVolume(volume, scale int64) (int64, error) {
	return (protocol.KlineInfo{VolumeScale: scale}).ToStorageVolume(volume)
}

func checkTickRule(i protocol.Instrument, operation string, rule protocol.TickRule) error {
	if err := checkSupport(i, operation, rule.Supported); err != nil {
		return err
	}
	if rule.Fields.Price && rule.PriceDivisor <= 0 {
		return fmt.Errorf("%s 的%s价格除数无效", i, operation)
	}
	_, _, err := tickVolumeAmount(0, rule)
	return err
}

func tickVolumeAmount(raw int64, rule protocol.TickRule) (int64, *float64, error) {
	var volume int64
	var amount *float64
	if rule.Fields.Volume {
		value, err := scaleVolume(raw, rule.VolumeScale)
		if err != nil {
			return 0, nil, err
		}
		volume = value
	}
	if rule.Fields.Amount {
		value, err := scaleVolume(raw, rule.AmountScale)
		if err != nil {
			return 0, nil, err
		}
		number := float64(value)
		amount = &number
	}
	return volume, amount, nil
}

func (q *Quote) scaleVolume(scale int64) (err error) {
	q.Volume, err = scaleVolume(q.Volume, scale)
	if err != nil {
		return err
	}
	for n := range q.BidVolume {
		q.BidVolume[n], err = scaleVolume(q.BidVolume[n], scale)
		if err != nil {
			return err
		}
		q.AskVolume[n], err = scaleVolume(q.AskVolume[n], scale)
		if err != nil {
			return err
		}
	}
	return nil
}

// applyRule removes unavailable quote fields before normalizing quantities.
func (q *Quote) applyRule(rule protocol.QuoteRule) error {
	if !rule.Fields.Price {
		q.PreClose, q.Open, q.High, q.Low, q.Price = 0, 0, 0, 0, 0
		q.Bid, q.Ask = [5]float64{}, [5]float64{}
	}
	if !rule.Fields.Volume {
		q.Volume, q.BidVolume, q.AskVolume = 0, [5]int64{}, [5]int64{}
		return nil
	}
	return q.scaleVolume(rule.VolumeScale)
}

func (k *Kline) applyFields(fields protocol.Fields) {
	if !fields.Price {
		k.Open, k.High, k.Low, k.Close = 0, 0, 0, 0
	}
	if !fields.Amount {
		k.Amount = nil
	}
}
