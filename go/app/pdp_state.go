package app

import "github.com/romayengineer/fme/go/store/pdpstate"

// FilePDPState adapts the JSON tombstone/miss files to the PDPState port.
type FilePDPState struct{}

// NewFilePDPState returns a PDPState backed by the PDP response directory.
func NewFilePDPState() *FilePDPState {
	return &FilePDPState{}
}

func (s *FilePDPState) MarkUnavailable(productID string) error {
	return pdpstate.MarkUnavailable(productID)
}

func (s *FilePDPState) ClearUnavailable(productID string) error {
	return pdpstate.ClearUnavailable(productID)
}

func (s *FilePDPState) LoadUnavailableProductIDs() map[string]struct{} {
	return pdpstate.LoadUnavailableProductIds()
}

func (s *FilePDPState) RecordPDPFailure(productID string, threshold int) (bool, int) {
	return pdpstate.RecordPDPFailure(productID, threshold)
}

func (s *FilePDPState) ClearPDPFailure(productID string) {
	pdpstate.ClearPDPFailure(productID)
}
