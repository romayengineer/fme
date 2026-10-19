package capture

// Sink persists intercepted responses and the products extracted from them.
// Capture depends on this interface rather than on the store package.
type Sink interface {
	SaveResponse(jsonDatas []any, friendlyName string) (int, error)
	SaveResponseByID(jsonDatas []any, friendlyName, id string) (int, error)
	MergeProducts(jsonDatas []any) int
	WriteDebugMiss(productID string, body []byte) error
}
