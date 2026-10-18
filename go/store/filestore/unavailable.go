package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/core/logging"
)

// DefaultMissThreshold is how many ambiguous "no product" PDP fetches a listing may
// accumulate before it is written off as no-longer-available.
const DefaultMissThreshold = 3

// State files live in the PDP response directory but use prefixes outside
// "response_"/"detail_" so ProcessData, GetFilePaths and DedupePDP ignore them.
const (
	unavailablePrefix = "unavailable_0_"
	missPrefix        = "miss_0_"
	debugDirName      = "debug"
)

func pdpDir() string {
	return filepath.Join(DataDir, pdpDirName)
}

// MarkUnavailable records that productId no longer exists so later pull_description
// runs skip it. The file is a tombstone: delete it (or use -retry-unavailable) to
// attempt the listing again.
func MarkUnavailable(productId string) error {
	record := map[string]any{
		"id":         productId,
		"detectedAt": time.Now().UTC().Format(time.RFC3339),
	}
	return writePDPState(unavailablePrefix+productId+".json", record)
}

// ClearUnavailable removes the tombstone for productId, e.g. after a successful
// fetch during a -retry-unavailable run.
func ClearUnavailable(productId string) error {
	err := os.Remove(filepath.Join(pdpDir(), unavailablePrefix+productId+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// LoadUnavailableProductIds returns the set of ids tombstoned as no-longer-available.
func LoadUnavailableProductIds() map[string]struct{} {
	ids := map[string]struct{}{}

	entries, err := os.ReadDir(pdpDir())
	if err != nil {
		return ids
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, unavailablePrefix) || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, unavailablePrefix), ".json")
		if id != "" {
			ids[id] = struct{}{}
		}
	}

	return ids
}

// RecordPDPFailure increments the ambiguous-failure counter for productId and, once
// it reaches threshold, writes the unavailable tombstone and clears the counter.
// It returns whether the listing just became unavailable and the current miss count.
func RecordPDPFailure(productId string, threshold int) (bool, int) {
	if threshold <= 0 {
		threshold = DefaultMissThreshold
	}

	count := 0
	missPath := filepath.Join(pdpDir(), missPrefix+productId+".json")
	if body, err := os.ReadFile(missPath); err == nil {
		var rec struct {
			Count int `json:"count"`
		}
		if json.Unmarshal(body, &rec) == nil {
			count = rec.Count
		}
	}
	count++

	if count >= threshold {
		if err := MarkUnavailable(productId); err != nil {
			logging.LogError0("RecordPDPFailure", "could not mark unavailable", "id", productId, "error", err)
		}
		_ = os.Remove(missPath)
		return true, count
	}

	record := map[string]any{
		"id":          productId,
		"count":       count,
		"lastFailure": time.Now().UTC().Format(time.RFC3339),
	}
	if err := writePDPState(missPrefix+productId+".json", record); err != nil {
		logging.LogError0("RecordPDPFailure", "could not write miss counter", "id", productId, "error", err)
	}
	return false, count
}

// ClearPDPFailure removes any miss counter for productId after a successful fetch.
func ClearPDPFailure(productId string) {
	_ = os.Remove(filepath.Join(pdpDir(), missPrefix+productId+".json"))
}

// WriteDebugMiss saves the raw body of a non-product PDP reply so the exact
// dead-listing payload can be inspected later (data/.../debug/miss_<id>.json).
func WriteDebugMiss(productId string, body []byte) error {
	path := filepath.Join(pdpDir(), debugDirName, "miss_"+productId+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0644)
}

func writePDPState(name string, v any) error {
	indented, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAndDirs(filepath.Join(pdpDir(), name), indented, 0644)
}
