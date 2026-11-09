package pull

import "github.com/injoyai/tdx/v2/extend/model"

// Stage identifies a terminal stage result. Market events have an empty Stage.
type Stage string

const (
	StageDay    Stage = "day"
	StageMinute Stage = "minute"
)

type Status string

const (
	StatusSkipped Status = "skipped"
	StatusNoData  Status = "no_data"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
)

type SkipReason string

const (
	SkipBeforeClose SkipReason = "before_close"
	SkipCompleted   SkipReason = "completed"
	SkipEmpty       SkipReason = "empty_universe"
	SkipCovered     SkipReason = "covered"
	SkipFuture      SkipReason = "future_start"
	SkipDayFailed   SkipReason = "day_failed"
)

// FetchResult describes the final attempt only, not cumulative retry traffic.
// Known=false means an older Unit does not supply metrics; zeros are then unknown.
// Fetched counts bars returned by the primary source scan to the save pipeline,
// before save-range filtering (not fallback trades/bars or protocol-page traffic).
// Saved counts rows in successful save calls, including replacements/fallback rows;
// it is not the number of newly inserted timestamps. Partial saves survive errors.
// LatestTime is the latest primary source time, without timezone conversion.
type FetchResult struct {
	Known      bool
	Fetched    int
	Saved      int
	LatestTime string
	SkipReason SkipReason
}

// ResultUnit optionally supplies metrics when OnEvent is configured, without
// changing the Unit method set. Its methods must perform the same work as
// FetchDay/FetchMinute. Wrappers overriding either legacy fetch method must also
// override its result method if they embed an existing ResultUnit.
type ResultUnit interface {
	Unit
	FetchDayResult(*Service, model.Code) (FetchResult, error)
	FetchMinuteResult(*Service, model.Code) (FetchResult, error)
}

// Event is a terminal market (Stage empty, Code zero) or per-code stage result.
// Stage events precede the legacy per-code OnProgress callback. Completed/Total
// count securities in the market, not stages; Completed advances when a code ends.
// Market events leave these counts zero. Stage callbacks see the current number
// of completed codes, before the code's own OnProgress notification.
// A failed market event can follow successful stages if saving its marker fails.
type Event struct {
	Market           string
	Code             model.Code
	Stage            Stage
	Status           Status
	Reason           SkipReason
	Completed, Total int
	Attempts         int
	Result           FetchResult
	Err              error
}

func (s *Service) emit(event Event) {
	if callback := s.cfg.OnEvent; callback != nil {
		callback(event)
	}
}

func stageStatus(result FetchResult, err error) (Status, SkipReason) {
	if err != nil {
		return StatusFailed, ""
	}
	if result.SkipReason != "" {
		return StatusSkipped, result.SkipReason
	}
	if result.Known && result.Fetched == 0 && result.Saved == 0 {
		return StatusNoData, ""
	}
	return StatusSuccess, ""
}
