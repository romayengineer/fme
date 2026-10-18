package filestore

import (
	"fmt"

	"github.com/romayengineer/fme/go/core/jsonutil"
)

// DescriptionStats summarizes how many detail records have a description.
// A listing counts as empty only when Description is JSON null; an empty string
// counts as present. Listings whose id has been tombstoned as no-longer-available
// are counted separately and are not reported as empty.
type DescriptionStats struct {
	Total           int
	WithDescription int
	Unavailable     int
	Empty           int
}

func (s DescriptionStats) EmptyPercent() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Empty) / float64(s.Total) * 100
}

func (s DescriptionStats) WithDescriptionPercent() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.WithDescription) / float64(s.Total) * 100
}

func (s DescriptionStats) UnavailablePercent() float64 {
	if s.Total == 0 {
		return 0
	}
	return float64(s.Unavailable) / float64(s.Total) * 100
}

// ComputeDescriptionStats walks data/__Details and counts records by whether their
// Description is null, excluding listings already marked as no-longer-available
// (see LoadUnavailableProductIds).
func ComputeDescriptionStats() DescriptionStats {
	stats := DescriptionStats{}
	unavailable := LoadUnavailableProductIds()

	ForEachDetail(func(_ string, jsonData map[string]any) bool {
		stats.Total++

		id, _ := jsonutil.GetKey(jsonData, "ID").(string)
		if _, gone := unavailable[id]; gone {
			stats.Unavailable++
			return true
		}
		if jsonutil.GetKey(jsonData, "Description") == nil {
			stats.Empty++
		}
		return true
	}, false)

	stats.WithDescription = stats.Total - stats.Empty - stats.Unavailable
	return stats
}

// PrintDescriptionStats writes a human-readable description report to stdout.
func PrintDescriptionStats(stats DescriptionStats) {
	fmt.Println("data/__Details description report")
	fmt.Printf("  total listings:    %d\n", stats.Total)
	fmt.Printf("  with description:  %d (%.2f%%)\n", stats.WithDescription, stats.WithDescriptionPercent())
	fmt.Printf("  unavailable:       %d (%.2f%%)  deleted listings (skipped by pull-description)\n", stats.Unavailable, stats.UnavailablePercent())
	fmt.Printf("  empty (null):      %d (%.2f%%)\n", stats.Empty, stats.EmptyPercent())
}
