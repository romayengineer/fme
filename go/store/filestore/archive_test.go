package filestore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/romayengineer/fme/go/store/paths"
)

const testSearchQuery = "CometMarketplaceSearchContentPaginationQuery"

func TestArchiveSearchResponsesMovesOnlyOldSearchFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now()
	friendlyDir := filepath.Join(paths.DataDir, testSearchQuery)

	oldPath := writeTestResponse(t, friendlyDir, now.Add(-10*24*time.Hour))
	recentPath := writeTestResponse(t, friendlyDir, now.Add(-1*time.Hour))
	pdpPath := writeTestFile(t, paths.PDPDir(), "response_0_123.json")

	archived, pruned, err := ArchiveSearchResponses(now.Add(-7*24*time.Hour), now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("ArchiveSearchResponses: %v", err)
	}

	assertArchivedCounts(t, archived, pruned, 1, 0)
	assertFileMissing(t, oldPath)
	assertFilePresent(t, recentPath)
	assertFilePresent(t, pdpPath)
	assertFilePresent(t, filepath.Join(paths.ArchivedDir(), testSearchQuery, filepath.Base(oldPath)))
}

func TestArchiveSearchResponsesIsIdempotent(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now()
	writeTestResponse(t, filepath.Join(paths.DataDir, testSearchQuery), now.Add(-10*24*time.Hour))
	cutoff := now.Add(-7 * 24 * time.Hour)
	pruneBefore := now.Add(-30 * 24 * time.Hour)

	if _, _, err := ArchiveSearchResponses(cutoff, pruneBefore); err != nil {
		t.Fatalf("first ArchiveSearchResponses: %v", err)
	}

	archived, pruned, err := ArchiveSearchResponses(cutoff, pruneBefore)
	if err != nil {
		t.Fatalf("second ArchiveSearchResponses: %v", err)
	}
	assertArchivedCounts(t, archived, pruned, 0, 0)
}

func TestArchiveSearchResponsesPrunesOldArchives(t *testing.T) {
	t.Chdir(t.TempDir())
	now := time.Now()
	oldArchive := writeTestFile(t, filepath.Join(paths.ArchivedDir(), testSearchQuery), "response_1_000001.json")
	recentArchive := writeTestFile(t, filepath.Join(paths.ArchivedDir(), testSearchQuery), "response_2_000002.json")

	if err := os.Chtimes(oldArchive, now.Add(-40*24*time.Hour), now.Add(-40*24*time.Hour)); err != nil {
		t.Fatalf("set old archive mtime: %v", err)
	}

	archived, pruned, err := ArchiveSearchResponses(now, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("ArchiveSearchResponses: %v", err)
	}
	assertArchivedCounts(t, archived, pruned, 0, 1)
	assertFileMissing(t, oldArchive)
	assertFilePresent(t, recentArchive)
}

func TestArchiveSearchResponsesWithoutDataDir(t *testing.T) {
	t.Chdir(t.TempDir())

	archived, pruned, err := ArchiveSearchResponses(time.Now(), time.Now())
	if err != nil {
		t.Fatalf("ArchiveSearchResponses: %v", err)
	}
	assertArchivedCounts(t, archived, pruned, 0, 0)
}

// writeTestResponse writes a timestamped response file and returns its path.
func writeTestResponse(t *testing.T, dir string, at time.Time) string {
	t.Helper()

	name := filepath.Base(paths.ResponseFile(filepath.Base(dir), paths.ResponsePrefix, at.UnixNano(), 1))
	return writeTestFile(t, dir, name)
}

// writeTestFile creates a JSON file and returns its path.
func writeTestFile(t *testing.T, dir, name string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("create dir %s: %v", dir, err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// assertArchivedCounts compares the archive and prune counters.
func assertArchivedCounts(t *testing.T, gotArchived, gotPruned, wantArchived, wantPruned int) {
	t.Helper()

	if gotArchived != wantArchived {
		t.Errorf("archived = %d, want %d", gotArchived, wantArchived)
	}
	if gotPruned != wantPruned {
		t.Errorf("pruned = %d, want %d", gotPruned, wantPruned)
	}
}

// assertFileMissing fails when path still exists.
func assertFileMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists (err = %v)", path, err)
	}
}

// assertFilePresent fails when path is missing.
func assertFilePresent(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("%s is missing: %v", path, err)
	}
}
