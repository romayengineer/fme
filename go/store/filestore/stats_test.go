package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/romayengineer/fme/go/store/pdpstate"
)

// newDetailDir switches to a temporary working directory and creates the detail
// directory inside it.
func newDetailDir(t *testing.T) string {
	t.Helper()

	t.Chdir(t.TempDir())
	dir := filepath.Join(DataDir, "__Details")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return dir
}

// writeDetail writes one detail record fixture.
func writeDetail(t *testing.T, dir, id string, description any) {
	t.Helper()

	writeDetailRecord(t, dir, id, map[string]any{"ID": id, "Description": description})
}

// writeDetailRecord writes an arbitrary detail record fixture.
func writeDetailRecord(t *testing.T, dir, id string, record map[string]any) {
	t.Helper()

	body, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "detail_"+id+".json"), body, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// readDetailRecord reads one detail record fixture.
func readDetailRecord(t *testing.T, dir, id string) map[string]any {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(dir, "detail_"+id+".json")) //nolint:gosec // temp test path.
	if err != nil {
		t.Fatalf("read detail: %v", err)
	}

	var record map[string]any
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	return record
}

// mustComputeStats computes the description stats or fails the test.
func mustComputeStats(t *testing.T) DescriptionStats {
	t.Helper()

	stats, err := ComputeDescriptionStats()
	if err != nil {
		t.Fatalf("ComputeDescriptionStats: %v", err)
	}
	return stats
}

// assertStatsCounts checks the integer counters of a stats report.
func assertStatsCounts(t *testing.T, stats DescriptionStats, total, empty, withDescription int) {
	t.Helper()

	if stats.Total != total {
		t.Errorf("Total = %d, want %d", stats.Total, total)
	}
	if stats.Empty != empty {
		t.Errorf("Empty = %d, want %d", stats.Empty, empty)
	}
	if stats.WithDescription != withDescription {
		t.Errorf("WithDescription = %d, want %d", stats.WithDescription, withDescription)
	}
}

// assertPercent checks one percentage field of a stats report.
func assertPercent(t *testing.T, name string, got, want float64) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestComputeDescriptionStats(t *testing.T) {
	detailDir := newDetailDir(t)

	writeDetail(t, detailDir, "1", "a real description")
	writeDetail(t, detailDir, "2", "")
	writeDetail(t, detailDir, "3", nil)
	writeDetail(t, detailDir, "4", nil)

	stats := mustComputeStats(t)
	assertStatsCounts(t, stats, 4, 2, 2)
	assertPercent(t, "EmptyPercent", stats.EmptyPercent(), 50)
	assertPercent(t, "WithDescriptionPercent", stats.WithDescriptionPercent(), 50)
}

func TestComputeDescriptionStatsUnavailable(t *testing.T) {
	detailDir := newDetailDir(t)

	writeDetail(t, detailDir, "1", "a real description")
	writeDetail(t, detailDir, "2", nil)

	if err := pdpstate.MarkUnavailable("2"); err != nil {
		t.Fatalf("MarkUnavailable: %v", err)
	}

	stats := mustComputeStats(t)
	assertStatsCounts(t, stats, 2, 0, 1)
	if stats.Unavailable != 1 {
		t.Errorf("Unavailable = %d, want 1", stats.Unavailable)
	}
}

func TestPendingDescriptionCount(t *testing.T) {
	detailDir := newDetailDir(t)

	writeDetail(t, detailDir, "1", "has text")
	writeDetail(t, detailDir, "2", "")  // empty string counts as done
	writeDetail(t, detailDir, "3", nil) // already fetched
	writeDetail(t, detailDir, "4", nil) // unavailable
	writeDetail(t, detailDir, "5", nil) // pending

	existing := map[string]struct{}{"3": {}}
	unavailable := map[string]struct{}{"4": {}}

	pending, alreadyFetched, unavailableCount, err := PendingDescriptionCount(existing, unavailable)
	if err != nil {
		t.Fatalf("PendingDescriptionCount: %v", err)
	}
	if pending != 1 || alreadyFetched != 1 || unavailableCount != 1 {
		t.Errorf("got pending=%d alreadyFetched=%d unavailable=%d, want 1/1/1",
			pending, alreadyFetched, unavailableCount)
	}
}

func TestComputeDescriptionStatsEmpty(t *testing.T) {
	newDetailDir(t)

	stats := mustComputeStats(t)
	assertStatsCounts(t, stats, 0, 0, 0)
	assertPercent(t, "EmptyPercent", stats.EmptyPercent(), 0)
	assertPercent(t, "WithDescriptionPercent", stats.WithDescriptionPercent(), 0)
}
