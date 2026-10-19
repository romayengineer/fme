package app

import "github.com/romayengineer/fme/go/store/filestore"

// FilePDPState adapts the JSON tombstone/miss files to the PDPState port.
type FilePDPState struct{}

// NewFilePDPState returns a PDPState backed by the PDP response directory.
func NewFilePDPState() *FilePDPState {
	return &FilePDPState{}
}

func (s *FilePDPState) MarkUnavailable(productID string) error {
	return filestore.MarkUnavailable(productID)
}

func (s *FilePDPState) ClearUnavailable(productID string) error {
	return filestore.ClearUnavailable(productID)
}

func (s *FilePDPState) LoadUnavailableProductIDs() map[string]struct{} {
	return filestore.LoadUnavailableProductIds()
}

func (s *FilePDPState) RecordPDPFailure(productID string, threshold int) (bool, int) {
	return filestore.RecordPDPFailure(productID, threshold)
}

func (s *FilePDPState) ClearPDPFailure(productID string) {
	filestore.ClearPDPFailure(productID)
}
