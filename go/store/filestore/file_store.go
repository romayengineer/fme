package filestore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/romayengineer/fme/go/marketplace/model"
)

const DataDir = "data"

// detailMu serializes every detail-file read-modify-write. Each Save creates a
// fresh FileStoreImpl, so a per-instance mutex cannot protect the same file from
// concurrent writers (capture, FillEmpty and replayed PDP merges).
var detailMu sync.RWMutex

type FileStoreImpl[T any] struct {
	id       string
	fileName string
	fileDir  string
	data     *T
}

type FileStore[T any] interface {
	SetDir(dir string)
	Get() (*T, error)
	Save(data T) (*T, error)
}

func NewProductFileStore(productId string) FileStore[model.MarketplaceItemDetails] {
	fileName := fmt.Sprintf("detail_%v.json", productId)
	fileDir := fmt.Sprintf("%v/__Details", DataDir)
	return &FileStoreImpl[model.MarketplaceItemDetails]{id: productId, fileName: fileName, fileDir: fileDir}
}

func (pfs *FileStoreImpl[T]) SetDir(dir string) {
	pfs.fileDir = dir
}

func (pfs *FileStoreImpl[T]) Get() (*T, error) {
	detailMu.RLock()
	defer detailMu.RUnlock()

	return pfs.getLocked()
}

// getLocked reads the file without taking detailMu. Callers must already hold it.
func (pfs *FileStoreImpl[T]) getLocked() (*T, error) {
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

func (pfs *FileStoreImpl[T]) Save(data T) (*T, error) {
	detailMu.Lock()
	defer detailMu.Unlock()

	if pfs.data == nil {
		_, err := pfs.getLocked()
		if err != nil {
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

	indented, err := json.MarshalIndent(pfs.data, "", "  ")
	if err != nil {
		return nil, err
	}

	filePath := filepath.Join(pfs.fileDir, pfs.fileName)
	if err := WriteFileAndDirs(filePath, indented, 0644); err != nil {
		return nil, err
	}

	return pfs.data, nil
}
