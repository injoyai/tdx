// Package storage defines persistence contracts without selecting a database.
package storage

import (
	"context"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// CodesStore atomically replaces the specified exchanges and their refresh state.
// No other exchanges or historical bars/events may be deleted. Returned records are caller-owned.
type CodesStore interface {
	LoadCodes(context.Context, []protocol.Exchange) (model.CodeSnapshot, error)
	ReplaceCodes(context.Context, model.CodeSnapshot) error
}

// GbbqStore replaces complete event histories and refresh state for the specified securities.
type GbbqStore interface {
	LoadGbbq(context.Context, []string) (model.GbbqSnapshot, error)
	ReplaceGbbq(context.Context, model.GbbqSnapshot) error
}

// WorkdayStore atomically merges only adjacent/overlapping source coverage ranges.
// A conflicting market/source or a disjoint range must fail rather than claim unobserved coverage.
type WorkdayStore interface {
	LoadWorkdays(context.Context, protocol.Market) (model.WorkdaySnapshot, error)
	SaveWorkdays(context.Context, model.WorkdaySnapshot) error
}
