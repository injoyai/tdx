package tdx

import (
	"fmt"

	"github.com/injoyai/tdx/v2/protocol"
)

// volumeScale 使用独立的报价/分时/分笔配置；国内默认100由配置显式声明。
func volumeScale(i protocol.Instrument) (int64, error) {
	m, err := i.Market()
	if err != nil {
		return 0, err
	}
	info, ok := m.Info()
	if ok && m != protocol.MarketHKIndex && info.TickVolumeScale > 0 {
		return info.TickVolumeScale, nil
	}
	return 0, fmt.Errorf("%s 的报价、分时或分笔字段不能确认为成交数量，不能转换为最小单位", i)
}

func klineVolumeScale(i protocol.Instrument, category uint8, info protocol.MarketInfo) (int64, error) {
	scale := info.Day.VolumeScale
	if category == info.Minute.Category {
		scale = info.Minute.VolumeScale
	}
	m, err := i.Market()
	if err != nil {
		return 0, err
	}
	if scale > 0 && m != protocol.MarketHKIndex {
		return scale, nil
	}
	return 0, fmt.Errorf("%s 周期 %d 的成交量单位尚未确认，不能转换为最小单位", i, category)
}

func scaleVolume(volume, scale int64) (int64, error) {
	return (protocol.KlineInfo{VolumeScale: scale}).ToStorageVolume(volume)
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
