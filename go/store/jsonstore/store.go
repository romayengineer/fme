// Package jsonstore provides atomic JSON file storage and the generic
// read-modify-write store used for product detail records.
package jsonstore

import (
	"encoding/json"
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
	SetDir(dir string)
	Get() (*T, error)
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
	content, err := os.ReadFile(filePath)
	if err != nil {
		pfs.data = &data
		return pfs.data, nil
	}

	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	pfs.data = &data
	return pfs.data, nil
}

func (pfs *fileStoreImpl[T]) Save(data T) (*T, error) {
	detailMu.Lock()
	defer detailMu.Unlock()

	if pfs.data == nil {
		if _, err := pfs.getLocked(); err != nil {
			return nil, err
		}
	}

	pfsValue := reflect.ValueOf(pfs.data).Elem()
	dataValue := reflect.ValueOf(data)

	for i := 0; i < pfsValue.NumField(); i++ {
		pfsField := pfsValue.Field(i)
		dataField := dataValue.Field(i)

		if !dataField.IsNil() {
			pfsField.Set(dataField)
		}
	}

	filePath := filepath.Join(pfs.fileDir, pfs.fileName)
	if err := WriteJSONFile(filePath, pfs.data, 0o644); err != nil {
		return nil, err
	}

	return pfs.data, nil
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
	if err := os.MkdirAll(nameDir, 0o755); err != nil {
		return err
	}
	return writeFileAtomic(name, data, perm)
}

// writeFileAtomic writes data to a temporary file in the destination directory
// and renames it into place, so readers never observe a partially written file
// and a crash leaves the previous version intact.
func writeFileAtomic(name string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(name)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", name, err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file for %s: %w", name, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file for %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file for %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file for %s: %w", name, err)
	}
	if err := os.Rename(tmpName, name); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp file to %s: %w", name, err)
	}

	return nil
}
