package model

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

type Gbbq = protocol.Gbbq

// CodeSnapshot replaces only the listed standard exchanges, including an empty result.
// FetchedAt describes refresh state, not the time of any market observation.
type CodeSnapshot struct {
	Exchanges []protocol.Exchange
	Rows      []*CodeModel
	FetchedAt time.Time
}

func (s CodeSnapshot) Validate() error {
	if s.FetchedAt.IsZero() || len(s.Exchanges) == 0 {
		return fmt.Errorf("model: code snapshot needs scope and refresh time")
	}
	scope := map[string]bool{}
	for _, ex := range s.Exchanges {
		if ex > protocol.ExchangeBJ || scope[ex.String()] {
			return fmt.Errorf("model: invalid or duplicate standard exchange %s", ex)
		}
		scope[ex.String()] = true
	}
	if err := ValidateCodes(s.Rows); err != nil {
		return err
	}
	for _, row := range s.Rows {
		if !scope[row.Exchange] {
			return fmt.Errorf("model: security outside snapshot scope %s", row.FullCode())
		}
	}
	return nil
}

// GbbqSnapshot replaces complete event histories only for Codes. Unrelated histories survive.
type GbbqSnapshot struct {
	Codes     []string
	Rows      []*Gbbq
	FetchedAt time.Time
}

func (s GbbqSnapshot) Validate() error {
	if s.FetchedAt.IsZero() || len(s.Codes) == 0 {
		return fmt.Errorf("model: event snapshot needs securities and refresh time")
	}
	scope := map[string]bool{}
	for _, code := range s.Codes {
		i, err := protocol.ParseInstrument(code)
		if err != nil || i.Exchange > protocol.ExchangeBJ || i.Exchange.String()+i.Code != code || scope[code] {
			return fmt.Errorf("model: invalid or duplicate event security %s", code)
		}
		scope[code] = true
	}
	for _, row := range s.Rows {
		if row == nil || !scope[row.Code] || row.Time.IsZero() {
			return fmt.Errorf("model: event outside snapshot scope")
		}
	}
	return nil
}

// WorkdaySnapshot records observed dates in an inclusive source coverage range.
// It is not a forecast of future exchange holidays.
type WorkdaySnapshot struct {
	Market    protocol.Market
	Source    string
	From      string
	Through   string
	Dates     []string
	FetchedAt time.Time
}

func (s WorkdaySnapshot) Validate() error {
	if s.Market == "" || s.Source == "" {
		return fmt.Errorf("model: calendar needs market and source")
	}
	if (s.From == "") != (s.Through == "") || s.From > s.Through {
		return fmt.Errorf("model: invalid calendar range")
	}
	for _, date := range append([]string{s.From, s.Through}, s.Dates...) {
		if date == "" && len(s.Dates) == 0 && s.From == "" {
			continue
		}
		t, err := time.Parse(time.DateOnly, date)
		if err != nil || t.Format(time.DateOnly) != date || date < s.From || date > s.Through {
			return fmt.Errorf("model: invalid calendar date %q", date)
		}
	}
	seen := map[string]bool{}
	for _, date := range s.Dates {
		if seen[date] {
			return fmt.Errorf("model: duplicate calendar date %s", date)
		}
		seen[date] = true
	}
	return nil
}
