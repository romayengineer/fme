package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type testRecord struct {
	Name  *string
	Count *int
	Flag  *bool
}

func strPtr(value string) *string { return &value }
func intPtr(value int) *int       { return &value }

// newTestStore returns a store rooted in a temp directory.
func newTestStore(t *testing.T) FileStore[testRecord] {
	t.Helper()
	return NewFileStore[testRecord]("1", "detail_1.json", t.TempDir())
}

func TestFileStoreGetMissingReturnsZeroRecord(t *testing.T) {
	store := newTestStore(t)

	record, err := store.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if record == nil || record.Name != nil || record.Count != nil {
		t.Fatalf("Get = %+v, want a zero record", record)
	}
}

func TestFileStoreSaveMergesNonNilFields(t *testing.T) {
	store := newTestStore(t)

	if _, err := store.Save(testRecord{Name: strPtr("first"), Count: intPtr(1)}); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if _, err := store.Save(testRecord{Name: strPtr("second")}); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	record, err := store.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertMergedRecord(t, record)
}

// assertMergedRecord checks the updated name and preserved count.
func assertMergedRecord(t *testing.T, record *testRecord) {
	t.Helper()

	if record.Name == nil || *record.Name != "second" {
		t.Errorf("Name = %v, want second", record.Name)
	}
	if record.Count == nil || *record.Count != 1 {
		t.Errorf("Count = %v, want the merged value 1", record.Count)
	}
}

func TestFileStoreConcurrentSavesKeepValidJSON(t *testing.T) {
	dir := t.TempDir()
	newStore := func() FileStore[testRecord] {
		return NewFileStore[testRecord]("1", "detail_1.json", dir)
	}

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			if _, err := newStore().Save(testRecord{Count: intPtr(i)}); err != nil {
				t.Errorf("Save %d: %v", i, err)
			}
		})
	}
	wg.Wait()

	record, err := newStore().Get()
	if err != nil {
		t.Fatalf("Get after concurrent saves: %v", err)
	}
	if record.Count == nil {
		t.Fatal("Count is nil after concurrent saves")
	}
}

func TestWriteFileAndDirsAtomicPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "value.json")
	if err := WriteFileAndDirs(path, []byte(`{"ok":true}`), 0o600); err != nil {
		t.Fatalf("WriteFileAndDirs: %v", err)
	}

	assertFileMode(t, path, 0o600)
	assertFileOK(t, path)
}

// assertFileMode checks the exact permission bits.
func assertFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode = %o, want %o", got, want)
	}
}

// assertFileOK checks the JSON payload.
func assertFileOK(t *testing.T, path string) {
	t.Helper()

	var got map[string]bool
	body, err := os.ReadFile(path) //nolint:gosec // temp test path.
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got["ok"] {
		t.Errorf("content = %v, want ok=true", got)
	}
}
