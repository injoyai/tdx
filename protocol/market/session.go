package market

import "fmt"

// TradingSession describes the intraday clock used by layouts that derive
// minute timestamps from record order. Values are minutes after midnight and
// mark the open, the end of the morning bars, and the end of a lunch break.
// Schedule fields on MarketInfo do not interpret source timestamps; a session
// is only consulted when a layout carries no per-record time.
type TradingSession struct {
	Open       int // 连续竞价开始，即首根分钟之前的时刻
	LunchStart int // 上午最后一根分钟的结束时刻；0 表示无午休
	LunchEnd   int // 下午第一根分钟之前的时刻
}

// DomesticSession is the shared Shanghai/Shenzhen/Beijing continuous session:
// 09:30-11:30 and 13:00-15:00, producing 09:31-11:30 and 13:01-15:00 bars.
var DomesticSession = TradingSession{Open: 570, LunchStart: 690, LunchEnd: 780}

// Valid reports whether the session can derive a bar clock from record order.
func (s TradingSession) Valid() bool {
	if s.Open <= 0 || s.Open >= 24*60 {
		return false
	}
	if s.LunchStart == 0 {
		return true
	}
	return s.LunchStart > s.Open && s.LunchStart < s.LunchEnd && s.LunchEnd < 24*60
}

// HasLunchBreak reports whether the session pauses between morning and afternoon bars.
func (s TradingSession) HasLunchBreak() bool { return s.LunchStart > 0 }

// AuctionMinute reports whether the 241-bar layout (09:30 auction plus
// 09:31-11:30 and 13:01-15:00) matches this session.
func (s TradingSession) AuctionMinute() bool { return s == DomesticSession }

// LunchTimeFix reports whether a 13:00 record maps back to 11:30 for
// this session, as the standard intraday K-line layout expects.
func (s TradingSession) LunchTimeFix() bool {
	return s.HasLunchBreak() && s.LunchStart == 690 && s.LunchEnd == 780
}

// MinuteOfDay derives the minute-of-day for the index-th bar after the open.
func (s TradingSession) MinuteOfDay(index int) (int, error) {
	if !s.Valid() {
		return 0, fmt.Errorf("无效交易时段 %+v", s)
	}
	if index < 0 {
		return 0, fmt.Errorf("分钟序号 %d 不能为负", index)
	}
	morning := s.LunchStart - s.Open
	var value int
	if s.HasLunchBreak() && index >= morning {
		value = s.LunchEnd + index - morning + 1
	} else {
		value = s.Open + index + 1
	}
	if value >= 24*60 {
		return 0, fmt.Errorf("分钟序号 %d 超出交易时段 %+v", index, s)
	}
	return value, nil
}

// HourMinute formats the index-th bar clock as HH:MM.
func (s TradingSession) HourMinute(index int) (string, error) {
	value, err := s.MinuteOfDay(index)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%02d:%02d", value/60, value%60), nil
}
