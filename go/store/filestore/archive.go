package filestore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/store/paths"
)

// ArchiveSearchResponses moves processed search responses older than cutoff
// into data/archived/, then deletes archived files older than pruneBefore.
// PDP responses (timestamp 0) and detail records are never touched, and only
// files older than cutoff are moved so recent unprocessed captures survive.
func ArchiveSearchResponses(cutoff, pruneBefore time.Time) (archived, pruned int, err error) {
	archived, err = archiveOldResponses(cutoff)
	if err != nil {
		return archived, 0, err
	}

	pruned, err = pruneArchived(pruneBefore)
	logging.LogInfo0("ArchiveSearchResponses", "finished", "archived", archived, "pruned", pruned)
	return archived, pruned, err
}

// archiveOldResponses moves old responses from every eligible data subdirectory.
func archiveOldResponses(cutoff time.Time) (int, error) {
	dirs, err := responseSourceDirs()
	if err != nil {
		return 0, err
	}

	total := 0
	for _, dir := range dirs {
		moved, err := archiveDir(dir, cutoff)
		if err != nil {
			return total, err
		}
		total += moved
	}
	return total, nil
}

// responseSourceDirs returns the data subdirectories that may hold search responses.
func responseSourceDirs() ([]string, error) {
	entries, err := os.ReadDir(paths.DataDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read data dir: %w", err)
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if isResponseSource(entry) {
			dirs = append(dirs, filepath.Join(paths.DataDir, entry.Name()))
		}
	}
	return dirs, nil
}

// isResponseSource reports whether entry is a directory that holds raw responses.
func isResponseSource(entry os.DirEntry) bool {
	if !entry.IsDir() {
		return false
	}

	switch entry.Name() {
	case paths.DetailDirName, paths.PDPDirName, paths.ArchivedDirName, "analytics":
		return false
	default:
		return true
	}
}

// archiveDir moves the old response files in one directory.
func archiveDir(dir string, cutoff time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("read response dir %s: %w", dir, err)
	}

	moved := 0
	for _, entry := range entries {
		if !isOldResponseFile(dir, entry, cutoff) {
			continue
		}
		if err := moveToArchive(dir, entry.Name()); err != nil {
			return moved, err
		}
		moved++
	}
	return moved, nil
}

// isOldResponseFile reports whether entry is a timestamped response older than cutoff.
func isOldResponseFile(dir string, entry os.DirEntry, cutoff time.Time) bool {
	if entry.IsDir() || !hasResponseName(entry.Name()) {
		return false
	}

	timestamp, err := paths.ParseTimestamp(filepath.Join(dir, entry.Name()))
	return err == nil && timestamp > 0 && time.Unix(0, timestamp).Before(cutoff)
}

// hasResponseName reports whether name is a raw response filename.
func hasResponseName(name string) bool {
	return strings.HasPrefix(name, paths.ResponsePrefix) && strings.HasSuffix(name, ".json")
}

// moveToArchive moves one response into data/archived/<friendlyName>/.
func moveToArchive(sourceDir, name string) error {
	targetDir := filepath.Join(paths.ArchivedDir(), filepath.Base(sourceDir))
	if err := os.MkdirAll(targetDir, 0o750); err != nil {
		return fmt.Errorf("create archive dir %s: %w", targetDir, err)
	}

	source := filepath.Join(sourceDir, name)
	target := filepath.Join(targetDir, name)
	if err := os.Rename(source, target); err != nil {
		return fmt.Errorf("archive %s: %w", source, err)
	}
	return nil
}

// pruneArchived deletes archived response files older than before.
func pruneArchived(before time.Time) (int, error) {
	if !archiveDirExists() {
		return 0, nil
	}

	pruned := 0
	err := filepath.WalkDir(paths.ArchivedDir(), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return pruneEntry(path, entry, before, &pruned)
	})
	if err != nil {
		return pruned, fmt.Errorf("walk archive dir: %w", err)
	}
	return pruned, nil
}

// archiveDirExists reports whether data/archived exists.
func archiveDirExists() bool {
	_, err := os.Stat(paths.ArchivedDir())
	return !errors.Is(err, os.ErrNotExist)
}

// pruneEntry removes one archived file when it is old enough.
func pruneEntry(path string, entry fs.DirEntry, before time.Time, pruned *int) error {
	if !shouldPruneArchived(entry, before) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("prune archived file %s: %w", path, err)
	}
	*pruned++
	return nil
}

// shouldPruneArchived reports whether entry is an archived file older than before.
func shouldPruneArchived(entry fs.DirEntry, before time.Time) bool {
	if entry.IsDir() {
		return false
	}

	info, err := entry.Info()
	if err != nil {
		return false
	}
	return info.ModTime().Before(before)
}
