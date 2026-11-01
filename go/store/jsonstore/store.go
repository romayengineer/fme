// Package jsonstore provides atomic JSON file storage and the generic
// read-modify-write store used for product detail records.
package jsonstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
)

// detailMu serializes every detail-file read-modify-write. Each Save uses a
// fresh store instance, so a per-instance mutex cannot protect the same file
// from concurrent writers.
var detailMu sync.RWMutex

// FileStore is a read-modify-write JSON store for one file.
type FileStore[T any] interface {
	// SetDir changes the directory the store reads and writes.
	SetDir(dir string)
	// Get returns the persisted record, or a zero record when the file is missing.
	Get() (*T, error)
	// Save merges the non-nil fields of data into the persisted record.
	Save(data T) (*T, error)
}

type fileStoreImpl[T any] struct {
	id       string
	fileName string
	fileDir  string
	data     *T
}

// NewFileStore returns a store for fileName inside dir.
func NewFileStore[T any](id, fileName, dir string) FileStore[T] {
	return &fileStoreImpl[T]{id: id, fileName: fileName, fileDir: dir}
}

func (pfs *fileStoreImpl[T]) SetDir(dir string) {
	pfs.fileDir = dir
}

func (pfs *fileStoreImpl[T]) Get() (*T, error) {
	detailMu.RLock()
	defer detailMu.RUnlock()

	return pfs.getLocked()
}

// getLocked reads the file without taking detailMu. Callers must already hold it.
func (pfs *fileStoreImpl[T]) getLocked() (*T, error) {
	var data T

	filePath := filepath.Join(pfs.fileDir, pfs.fileName)
	content, err := os.ReadFile(filePath) //nolint:gosec // path is built from the configured data root.
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", filePath, err)
		}
		pfs.data = &data
		return pfs.data, nil
	}

	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	pfs.data = &data
	return pfs.data, nil
}

// Save merges the non-nil fields of data into the persisted record.
func (pfs *fileStoreImpl[T]) Save(data T) (*T, error) {
	detailMu.Lock()
	defer detailMu.Unlock()

	if pfs.data == nil {
		if _, err := pfs.getLocked(); err != nil {
			return nil, err
		}
	}

	mergeNonNil(pfs.data, data)

	filePath := filepath.Join(pfs.fileDir, pfs.fileName)
	if err := WriteJSONFile(filePath, pfs.data, 0o600); err != nil {
		return nil, err
	}

	return pfs.data, nil
}

// mergeNonNil copies every non-nil field of src into dst. Both values must be
// pointers to the same struct type.
func mergeNonNil[T any](dst *T, src T) {
	dstValue := reflect.ValueOf(dst).Elem()
	srcValue := reflect.ValueOf(src)

	for i := range dstValue.NumField() {
		dstField := dstValue.Field(i)
		srcField := srcValue.Field(i)

		if !srcField.IsNil() {
			dstField.Set(srcField)
		}
	}
}

// WriteJSONFile marshals v with indentation and writes it atomically.
func WriteJSONFile(path string, v any, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAndDirs(path, data, perm)
}

// WriteFileAndDirs creates the parent directory and writes data atomically.
func WriteFileAndDirs(name string, data []byte, perm os.FileMode) error {
	nameDir := filepath.Dir(name)
	if err := os.MkdirAll(nameDir, 0o750); err != nil {
		return err
	}
	return writeFileAtomic(name, data, perm)
}

// writeFileAtomic writes data to a temporary file in the destination directory
// and renames it into place, so readers never observe a partially written file
// and a crash leaves the previous version intact.
func writeFileAtomic(name string, data []byte, perm os.FileMode) error {
	tmp, tmpName, err := createTempFile(name)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpName) }()

	if err := writeAndClose(tmp, data, perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := os.Rename(tmpName, name); err != nil {
		return fmt.Errorf("rename temp file to %s: %w", name, err)
	}

	return nil
}

// createTempFile creates the hidden temporary file used for an atomic write.
func createTempFile(name string) (*os.File, string, error) {
	tmp, err := os.CreateTemp(filepath.Dir(name), "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return nil, "", fmt.Errorf("create temp file for %s: %w", name, err)
	}
	return tmp, tmp.Name(), nil
}

// writeAndClose writes, chmods, syncs, and closes tmp.
func writeAndClose(tmp *os.File, data []byte, perm os.FileMode) error {
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file for %s: %w", tmp.Name(), err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("chmod temp file for %s: %w", tmp.Name(), err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file for %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", tmp.Name(), err)
	}
	return nil
}
