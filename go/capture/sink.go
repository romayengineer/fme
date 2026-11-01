package capture

// Sink persists intercepted responses and the products extracted from them.
// Capture depends on this interface rather than on the store package.
type Sink interface {
	// SaveResponse stores the response payloads under a generated filename.
	SaveResponse(jsonDatas []any, friendlyName string) (int, error)
	// SaveResponseByID stores the response payloads under an id-keyed filename.
	SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error)
	// MergeProducts merges the payloads' products into their detail records and
	// returns how many were saved.
	MergeProducts(jsonDatas []any) int
	// WriteDebugMiss saves a raw non-product payload for later inspection.
	WriteDebugMiss(productID string, body []byte) error
}
