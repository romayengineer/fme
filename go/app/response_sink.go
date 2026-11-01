package app

import (
	"github.com/romayengineer/fme/go/store/filestore"
	"github.com/romayengineer/fme/go/store/pdpstate"
)

// FileResponseSink adapts the JSON store to the capture Sink/ResponseSink ports.
type FileResponseSink struct{}

// NewFileResponseSink returns a sink backed by data/.
func NewFileResponseSink() *FileResponseSink {
	return &FileResponseSink{}
}

// SaveResponse stores the response payloads under a generated filename.
func (s *FileResponseSink) SaveResponse(jsonDatas []any, friendlyName string) (int, error) {
	return filestore.WriteJSONResponse(jsonDatas, friendlyName)
}

// SaveResponseByID stores the response payloads under an id-keyed filename.
func (s *FileResponseSink) SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error) {
	return filestore.WriteJSONResponseByID(jsonDatas, friendlyName, id)
}

// MergeProducts merges the payloads products into their detail records.
func (s *FileResponseSink) MergeProducts(jsonDatas []any) int {
	return filestore.SaveProductsFrom(jsonDatas)
}

// WriteDebugMiss saves a raw non-product payload for later inspection.
func (s *FileResponseSink) WriteDebugMiss(productID string, body []byte) error {
	return pdpstate.WriteDebugMiss(productID, body)
}
