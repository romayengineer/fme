package app

import "fmt"

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

func (s DescriptionStats) EmptyPercent() float64 { return percent(s.Empty, s.Total) }
func (s DescriptionStats) WithDescriptionPercent() float64 {
	return percent(s.WithDescription, s.Total)
}
func (s DescriptionStats) UnavailablePercent() float64 { return percent(s.Unavailable, s.Total) }

// ProcessResponses materializes captured responses into data/__Details.
func (a *App) ProcessResponses() error {
	if a.Details == nil {
		return fmt.Errorf("process-data: detail store is not configured")
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

// FillEmpty backfills missing URLs on existing detail files.
func (a *App) FillEmpty() error {
	if a.Details == nil {
		return fmt.Errorf("fill-empty: detail store is not configured")
	}
	return a.Details.FillEmpty()
}

// DescriptionStats reports description coverage for data/__Details.
func (a *App) DescriptionStats() (DescriptionStats, error) {
	if a.Details == nil {
		return DescriptionStats{}, fmt.Errorf("description-stats: detail store is not configured")
	}
	return a.Details.DescriptionStats()
}
