package app

import "github.com/romayengineer/fme/go/store/pdpstate"

// FilePDPState adapts the JSON tombstone/miss files to the PDPState port.
type FilePDPState struct{}

// NewFilePDPState returns a PDPState backed by the PDP response directory.
func NewFilePDPState() *FilePDPState {
	return &FilePDPState{}
}

// MarkUnavailable tombstones a listing as no longer available.
func (s *FilePDPState) MarkUnavailable(productID string) error {
	return pdpstate.MarkUnavailable(productID)
}

// ClearUnavailable removes a listing tombstone.
func (s *FilePDPState) ClearUnavailable(productID string) error {
	return pdpstate.ClearUnavailable(productID)
}

// LoadUnavailableProductIDs returns the tombstoned product ids.
func (s *FilePDPState) LoadUnavailableProductIDs() map[string]struct{} {
	return pdpstate.LoadUnavailableProductIDs()
}

// RecordPDPFailure counts an ambiguous failure for a listing.
func (s *FilePDPState) RecordPDPFailure(productID string, threshold int) (bool, int) {
	return pdpstate.RecordPDPFailure(productID, threshold)
}

// ClearPDPFailure resets a listing miss counter.
func (s *FilePDPState) ClearPDPFailure(productID string) {
	pdpstate.ClearPDPFailure(productID)
}
