package tdx

import (
	"fmt"
	"math"

	"github.com/injoyai/tdx/v2/protocol"
)

// volumeScale 只处理已确认的接口口径，不借用 K 线单位解释其他接口。
func volumeScale(i protocol.Instrument) (int64, error) {
	m, err := i.Market()
	if err != nil {
		return 0, err
	}
	switch {
	case m == protocol.MarketFuture, m == protocol.MarketHK, m == protocol.MarketUS:
		return 1, nil
	case i.Exchange <= protocol.ExchangeBJ && (protocol.IsStock(standardCode(i)) || protocol.IsETF(standardCode(i))):
		return protocol.SharesPerLot, nil
	default:
		return 0, fmt.Errorf("%s 的报价、分时或分笔成交量单位尚未确认，不能转换为最小单位", i)
	}
}

func klineVolumeScale(i protocol.Instrument, category uint8, info protocol.MarketInfo) (int64, error) {
	scale := info.Day.VolumeScale
	if category == info.Minute.Category {
		scale = info.Minute.VolumeScale
	}
	if scale > 0 && !protocol.IsConvertibleBond(standardCode(i)) {
		return scale, nil
	}
	return 0, fmt.Errorf("%s 周期 %d 的成交量单位尚未确认，不能转换为最小单位", i, category)
}

func scaleVolume(volume, scale int64) (int64, error) {
	if scale <= 0 || volume < 0 || volume > math.MaxInt64/scale {
		return 0, fmt.Errorf("成交量 %d 无法按倍率 %d 转换为最小单位", volume, scale)
	}
	return volume * scale, nil
}
