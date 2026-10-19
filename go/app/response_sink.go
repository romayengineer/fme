package app

import "github.com/romayengineer/fme/go/store/filestore"

// FileResponseSink adapts the JSON store to the capture Sink/ResponseSink ports.
type FileResponseSink struct{}

// NewFileResponseSink returns a sink backed by data/.
func NewFileResponseSink() *FileResponseSink {
	return &FileResponseSink{}
}

func (s *FileResponseSink) SaveResponse(jsonDatas []any, friendlyName string) (int, error) {
	return filestore.WriteJsonResponse(jsonDatas, friendlyName)
}

func (s *FileResponseSink) SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error) {
	return filestore.WriteJsonResponseById(jsonDatas, friendlyName, id)
}

func (s *FileResponseSink) MergeProducts(jsonDatas []any) int {
	return filestore.SaveProductsFrom(jsonDatas)
}

func (s *FileResponseSink) WriteDebugMiss(productID string, body []byte) error {
	return filestore.WriteDebugMiss(productID, body)
}
