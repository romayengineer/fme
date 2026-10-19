package filestore

import (
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/jsonstore"
	"github.com/romayengineer/fme/go/store/paths"
)

// DataDir is the root of the scraped data directory.
const DataDir = paths.DataDir

// FileStore is a read-modify-write store for one JSON file.
type FileStore[T any] = jsonstore.FileStore[T]

// NewProductFileStore returns the store for a product's detail_<id>.json.
func NewProductFileStore(productId string) FileStore[model.MarketplaceItemDetails] {
	return jsonstore.NewFileStore[model.MarketplaceItemDetails](
		productId,
		paths.DetailFileName(productId),
		paths.DetailDir(),
	)
}
