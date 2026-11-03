package app

import (
	"errors"
	"time"
)

// Raw-response retention defaults for the scheduler.
const (
	// DefaultArchiveAfter is how long processed search responses stay in the
	// data root before they are moved to data/archived/.
	DefaultArchiveAfter = 7 * 24 * time.Hour
	// DefaultKeepArchived is how long archived responses are kept before deletion.
	DefaultKeepArchived = 30 * 24 * time.Hour
)

// ArchiveOptions configures raw-response retention.
type ArchiveOptions struct {
	// OlderThan overrides DefaultArchiveAfter.
	OlderThan time.Duration
	// KeepArchive overrides DefaultKeepArchived.
	KeepArchive time.Duration
}

// DescriptionStats summarizes how many detail records have a description.
type DescriptionStats struct {
	Total           int
	WithDescription int
	Unavailable     int
	Empty           int
}

func percent(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// EmptyPercent returns the share of records whose Description is null.
func (s DescriptionStats) EmptyPercent() float64 { return percent(s.Empty, s.Total) }

// WithDescriptionPercent returns the share of records with a description.
func (s DescriptionStats) WithDescriptionPercent() float64 {
	return percent(s.WithDescription, s.Total)
}

// UnavailablePercent returns the share of records tombstoned as unavailable.
func (s DescriptionStats) UnavailablePercent() float64 { return percent(s.Unavailable, s.Total) }

// ProcessResponses materializes captured responses into data/__Details.
func (a *App) ProcessResponses() error {
	if a.Details == nil {
		return errors.New("process-data: detail store is not configured")
	}
	_, err := a.Details.ProcessResponses(0)
	return err
}

// DedupePDP removes duplicate PDP response files.
func (a *App) DedupePDP() {
	if a.Details != nil {
		a.Details.DedupePDP()
	}
}

// ArchiveResponses moves processed search responses into data/archived/ and
// prunes old archives. It never touches PDP responses or detail records.
func (a *App) ArchiveResponses(opts ArchiveOptions) (archived, pruned int, err error) {
	if a.Details == nil {
		return 0, 0, errors.New("archive-responses: detail store is not configured")
	}

	now := time.Now()
	return a.Details.ArchiveSearchResponses(now.Add(-durationOr(opts.OlderThan, DefaultArchiveAfter)), now.Add(-durationOr(opts.KeepArchive, DefaultKeepArchived)))
}

// durationOr returns value when positive, otherwise fallback.
func durationOr(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

// FillEmpty backfills missing URLs on existing detail files.
func (a *App) FillEmpty() error {
	if a.Details == nil {
		return errors.New("fill-empty: detail store is not configured")
	}
	return a.Details.FillEmpty()
}

// DescriptionStats reports description coverage for data/__Details.
func (a *App) DescriptionStats() (DescriptionStats, error) {
	if a.Details == nil {
		return DescriptionStats{}, errors.New("description-stats: detail store is not configured")
	}
	return a.Details.DescriptionStats()
}
