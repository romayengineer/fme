// Package pdpstate persists the pull-description bookkeeping for listings that
// are temporarily failing or no longer available.
package pdpstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/store/jsonstore"
	"github.com/romayengineer/fme/go/store/paths"
)

// DefaultMissThreshold is how many ambiguous "no product" PDP fetches a listing may
// accumulate before it is written off as no-longer-available.
const DefaultMissThreshold = 3

// State files live in the PDP response directory but use prefixes outside
// "response_"/"detail_" so ProcessData, GetFilePaths and DedupePDP ignore them.
const (
	unavailablePrefix = "unavailable_0_"
	missPrefix        = "miss_0_"
)

func pdpDir() string {
	return paths.PDPDir()
}

// MarkUnavailable records that productID no longer exists so later pull_description
// runs skip it. The file is a tombstone: delete it (or use -retry-unavailable) to
// attempt the listing again.
func MarkUnavailable(productID string) error {
	record := map[string]any{
		"id":         productID,
		"detectedAt": time.Now().UTC().Format(time.RFC3339),
	}
	return writePDPState(unavailablePrefix+productID+".json", record)
}

// ClearUnavailable removes the tombstone for productID, e.g. after a successful
// fetch during a -retry-unavailable run.
func ClearUnavailable(productID string) error {
	err := os.Remove(filepath.Join(pdpDir(), unavailablePrefix+productID+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// LoadUnavailableProductIDs returns the set of ids tombstoned as no-longer-available.
func LoadUnavailableProductIDs() map[string]struct{} {
	ids := map[string]struct{}{}

	entries, err := os.ReadDir(pdpDir())
	if err != nil {
		return ids
	}

	for _, entry := range entries {
		if id, ok := unavailableID(entry); ok {
			ids[id] = struct{}{}
		}
	}

	return ids
}

// unavailableID extracts the tombstoned product id from a state file entry.
func unavailableID(entry os.DirEntry) (string, bool) {
	if entry.IsDir() {
		return "", false
	}

	name := entry.Name()
	if !strings.HasPrefix(name, unavailablePrefix) || !strings.HasSuffix(name, ".json") {
		return "", false
	}

	id := strings.TrimSuffix(strings.TrimPrefix(name, unavailablePrefix), ".json")
	return id, id != ""
}

// RecordPDPFailure increments the ambiguous-failure counter for productID and, once
// it reaches threshold, writes the unavailable tombstone and clears the counter.
// It returns whether the listing just became unavailable and the current miss count.
func RecordPDPFailure(productID string, threshold int) (bool, int) {
	if threshold <= 0 {
		threshold = DefaultMissThreshold
	}

	count := readMissCount(productID) + 1
	if count >= threshold {
		markUnavailable(productID)
		_ = os.Remove(missPath(productID))
		return true, count
	}

	writeMissCount(productID, count)
	return false, count
}

// missPath is the counter file for one product id.
func missPath(productID string) string {
	return filepath.Join(pdpDir(), missPrefix+productID+".json")
}

// readMissCount returns the recorded miss count, or zero when it is unreadable.
func readMissCount(productID string) int {
	body, err := os.ReadFile(missPath(productID))
	if err != nil {
		return 0
	}

	var rec struct {
		Count int `json:"count"`
	}
	if json.Unmarshal(body, &rec) != nil {
		return 0
	}
	return rec.Count
}

// markUnavailable writes the tombstone, logging rather than failing the run.
func markUnavailable(productID string) {
	if err := MarkUnavailable(productID); err != nil {
		logging.LogError0("RecordPDPFailure", "could not mark unavailable", "id", productID, "error", err)
	}
}

// writeMissCount records the current miss count, logging rather than failing.
func writeMissCount(productID string, count int) {
	record := map[string]any{
		"id":          productID,
		"count":       count,
		"lastFailure": time.Now().UTC().Format(time.RFC3339),
	}
	if err := writePDPState(missPrefix+productID+".json", record); err != nil {
		logging.LogError0("RecordPDPFailure", "could not write miss counter", "id", productID, "error", err)
	}
}

// ClearPDPFailure removes any miss counter for productID after a successful fetch.
func ClearPDPFailure(productID string) {
	_ = os.Remove(filepath.Join(pdpDir(), missPrefix+productID+".json"))
}

// WriteDebugMiss saves the raw body of a non-product PDP reply so the exact
// dead-listing payload can be inspected later (data/.../debug/miss_<id>.json).
func WriteDebugMiss(productID string, body []byte) error {
	path := filepath.Join(paths.PDPDebugDir(), "miss_"+productID+".json")
	return jsonstore.WriteFileAndDirs(path, body, 0o600)
}

// writePDPState writes one state file under the PDP directory.
func writePDPState(name string, v any) error {
	return jsonstore.WriteJSONFile(filepath.Join(pdpDir(), name), v, 0o600)
}
