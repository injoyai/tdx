package tdx

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol"
)

// volumeScale 使用独立的报价/分时/分笔配置；国内默认100由配置显式声明。
func volumeScale(i protocol.Instrument) (int64, error) {
	info, ok := i.Info()
	if ok && info.TickVolumeScale > 0 {
		return info.TickVolumeScale, nil
	}
	return 0, fmt.Errorf("%s 的市场配置不提供报价、分时或分笔的成交数量", i)
}

// klineVolumeScale 返回 0 表示该周期的 K 线没有可用成交量。
func klineVolumeScale(i protocol.Instrument, category uint8, info protocol.MarketInfo) (int64, error) {
	if scale := info.KlineVolumeScale(category); scale >= 0 {
		return scale, nil
	}
	return 0, fmt.Errorf("%s 周期 %d 的成交量倍率无效", i, category)
}

func scaleVolume(volume, scale int64) (int64, error) {
	return (protocol.KlineInfo{VolumeScale: scale}).ToStorageVolume(volume)
}

func tickVolumeAmount(raw, volumeScale, amountScale int64) (int64, *float64, error) {
	if amountScale == 0 {
		volume, err := scaleVolume(raw, volumeScale)
		return volume, nil, err
	}
	value, err := scaleVolume(raw, amountScale)
	if err != nil {
		return 0, nil, err
	}
	amount := float64(value)
	return 0, &amount, nil
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
