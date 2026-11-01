// Package filestore persists scraped responses and merged product records
// under the data/ directory.
package filestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/extract"
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/jsonstore"
	"github.com/romayengineer/fme/go/store/paths"
)

// SaveProductsIfAny merges products into their detail records and reports
// whether there was at least one product to save.
func SaveProductsIfAny(products []model.MarketplaceItemDetails) bool {
	for _, product := range products {
		saveProductIfValid(product)
	}
	return len(products) > 0
}

// saveProductIfValid merges one product into its detail record.
func saveProductIfValid(product model.MarketplaceItemDetails) {
	productID, ok := product.ID.(string)
	if !ok || productID == "" {
		logging.LogWarn0("SaveProductsIfAny", "skipping product without a valid id", "id", product.ID)
		return
	}

	if _, err := NewProductFileStore(productID).Save(product); err != nil {
		logging.LogError0("SaveProductsIfAny", "error saving product", "id", productID, "error", err)
	}
}

// SaveProductsFrom extracts products from in-memory response JSON and merges them
// into data/__Details, so a freshly fetched PDP updates its detail record immediately
// instead of waiting for the next ProcessData pass.
func SaveProductsFrom(jsonDatas []any) int {
	productExtractors := extract.NewProductExtractors()
	saved := 0
	for _, jsonData := range jsonDatas {
		if SaveProductsIfAny(productExtractors.ExtractAll(jsonData)) {
			saved++
		}
	}
	return saved
}

// processState accumulates the counters for one ProcessData run.
type processState struct {
	startTime      int64
	extractors     extract.ProductExtractors
	lastFilePath   string
	filesProcessed int
	filesDeleted   int
	gotRateLimit   bool
}

// ProcessData extracts products from every captured response newer than
// startTime, deletes responses that yield no product, and returns the timestamp
// of the last processed file.
func ProcessData(startTime int64) (int64, error) {
	state := &processState{
		startTime:  startTime,
		extractors: extract.NewProductExtractors(),
	}

	filesRead, err := ForEachResponse(state.process, true)
	lastTimestamp, _ := GetTimestamp(state.lastFilePath)

	logging.LogInfo0("ProcessData", "all files processed",
		"lastTimestamp", lastTimestamp,
		"filesProcessedCounter", state.filesProcessed,
		"filesReadCounter", filesRead,
		"filesDeletedCounter", state.filesDeleted)

	if state.gotRateLimit {
		logging.LogWarn0("ProcessData", "got rate limit")
	}

	return lastTimestamp, err
}

// process handles one captured response file.
func (s *processState) process(filePath string, jsonData map[string]any) bool {
	s.lastFilePath = filePath

	if !s.shouldProcess(filePath) {
		return true
	}

	s.filesProcessed++

	if SaveProductsIfAny(s.extractors.ExtractAll(jsonData)) {
		return true
	}

	s.recordRateLimit(jsonData)
	s.deleteResponse(filePath)
	return true
}

// shouldProcess reports whether the file is newer than the run's start time.
func (s *processState) shouldProcess(filePath string) bool {
	fileTimestamp, err := GetTimestamp(filePath)
	if err != nil {
		return false
	}
	return fileTimestamp >= s.startTime
}

// recordRateLimit remembers that Facebook throttled the run.
func (s *processState) recordRateLimit(jsonData map[string]any) {
	if extract.IsErrorRateLimit(jsonData) {
		s.gotRateLimit = true
	}
}

// deleteResponse removes a response that yielded no product.
func (s *processState) deleteResponse(filePath string) {
	logging.LogDebug0("ProcessData", "no product found, deleting file", "path", filePath)
	if err := os.Remove(filePath); err != nil {
		logging.LogError0("ProcessData", "error deleting file", "path", filePath, "error", err)
	}
	s.filesDeleted++
}

// GetTimestamp reads the timestamp embedded in a response filename.
func GetTimestamp(filePath string) (int64, error) {
	timestamp, err := paths.ParseTimestamp(filePath)
	if err != nil {
		logging.LogWarn0("GetTimestamp", "could not read timestamp", "filePath", filePath, "error", err)
		return 0, err
	}
	return timestamp, nil
}

// WriteJSONResponse writes each JSON payload to its own timestamped response
// file and returns how many were written.
func WriteJSONResponse(jsonDatas []any, friendlyName string) (int, error) {
	jsonCounter := 0
	var err error
	for _, jsonData := range jsonDatas {
		if err = WriteRandomJSONFileIndented("response", friendlyName, jsonData); err != nil {
			return jsonCounter, err
		}
		jsonCounter += 1
	}

	return jsonCounter, nil
}

// WriteJSONResponseByID writes response JSON to a deterministic filename keyed by
// id, overwriting any previous file for the same id. This makes writes idempotent
// so re-running never accumulates duplicate responses for the same product.
//
// The name keeps the "<prefix>_<number>_<...>.json" shape because GetTimestamp
// parses the second underscore-separated field as the (numeric) timestamp.
func WriteJSONResponseByID(jsonDatas []any, friendlyName string, id string) (int, error) {
	counter := 0
	for _, jsonData := range jsonDatas {
		if err := WriteJSONFileDeterministic("response", friendlyName, id, jsonData); err != nil {
			return counter, err
		}
		counter++
	}
	return counter, nil
}

// WriteJSONFileDeterministic writes jsonData to the id-keyed response path.
func WriteJSONFileDeterministic(prefix string, friendlyName string, key string, jsonData any) error {
	filename := paths.ResponseFileByID(friendlyName, prefix, key)
	return jsonstore.WriteJSONFile(filename, jsonData, 0o600)
}

// WriteFileAndDirs writes data atomically, creating parent directories.
func WriteFileAndDirs(name string, data []byte, perm os.FileMode) error {
	return jsonstore.WriteFileAndDirs(name, data, perm)
}

// TimeNow returns the current time as a Unix nanosecond timestamp.
func TimeNow() int64 {
	return time.Now().UnixNano()
}

// TimeYesterday returns the same time one day ago as a Unix nanosecond timestamp.
func TimeYesterday() int64 {
	return time.Now().AddDate(0, 0, -1).UnixNano()
}

// WriteRandomJSONFile writes body to a new timestamped response file.
func WriteRandomJSONFile(prefix string, friendlyName string, body []byte) error {
	timestamp := TimeNow()
	random := rand.IntN(1000000) //nolint:gosec // only needs to make response filenames unique.

	filename := paths.ResponseFile(friendlyName, prefix, timestamp, random)

	return WriteFileAndDirs(filename, body, 0o600)
}

// WriteRandomJSONFileIndented writes jsonData, indented, to a new response file.
func WriteRandomJSONFileIndented(prefix string, friendlyName string, jsonData any) error {
	indented, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return err
	}
	return WriteRandomJSONFile(prefix, friendlyName, indented)
}

// GetFilePaths returns the data files whose names start with prefix. When sortIt
// is true they are ordered by filename, otherwise they are shuffled.
func GetFilePaths(prefix string, sortIt bool) ([]string, error) {
	filePaths, err := walkDataFiles(prefix)
	if err != nil {
		return nil, err
	}

	orderFiles(filePaths, sortIt)

	return filePaths, nil
}

// walkDataFiles collects the non-archived data files with the given prefix.
func walkDataFiles(prefix string) ([]string, error) {
	filePaths := []string{}
	visit := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		filePaths = appendDataFile(filePaths, path, d, prefix)
		return nil
	}

	if err := filepath.WalkDir(DataDir, visit); err != nil {
		return nil, fmt.Errorf("walk %s: %w", DataDir, err)
	}
	return filePaths, nil
}

// appendDataFile appends path when it is a matching, non-archived data file.
func appendDataFile(filePaths []string, path string, d fs.DirEntry, prefix string) []string {
	if d.IsDir() || strings.Contains(path, "archived") {
		return filePaths
	}

	name := d.Name()
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".json") {
		return filePaths
	}
	return append(filePaths, path)
}

// orderFiles sorts filePaths by filename, or shuffles them when sortIt is false.
func orderFiles(filePaths []string, sortIt bool) {
	if sortIt {
		sort.Slice(filePaths, func(i, j int) bool {
			return filepath.Base(filePaths[i]) < filepath.Base(filePaths[j])
		})
		return
	}

	// Random order spreads the load when a run is stopped part-way.
	rand.Shuffle(len(filePaths), func(i, j int) { filePaths[i], filePaths[j] = filePaths[j], filePaths[i] })
}

// ForEachJSONInData calls process for every data file whose name starts with
// prefix, stopping early when process returns false. Read and parse failures are
// accumulated and returned together.
func ForEachJSONInData(prefix string, process func(filePath string, jsonData map[string]any) bool, sortIt bool) (int, error) {
	filesReadCounter := 0
	var errs []error

	filePaths, err := GetFilePaths(prefix, sortIt)
	if err != nil {
		return 0, err
	}

	for _, filePath := range filePaths {
		jsonData, err := readJSONObject(filePath)
		if err != nil {
			logging.LogError0("ForEachJSONInData", "could not read file", "path", filePath, "error", err)
			errs = append(errs, err)
			continue
		}

		shouldContinue := process(filePath, jsonData)

		filesReadCounter++

		if !shouldContinue {
			return filesReadCounter, errors.Join(errs...)
		}
	}

	return filesReadCounter, errors.Join(errs...)
}

// readJSONObject reads one file and decodes it as a JSON object.
func readJSONObject(filePath string) (map[string]any, error) {
	body, err := os.ReadFile(filePath) //nolint:gosec // path comes from the configured data root.
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	var jsonData map[string]any
	if err := json.Unmarshal(body, &jsonData); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}
	return jsonData, nil
}

// ForEachResponse calls process for every captured response file.
func ForEachResponse(process func(filePath string, jsonData map[string]any) bool, sortIt bool) (int, error) {
	return ForEachJSONInData("response_", process, sortIt)
}

// ForEachDetail calls process for every merged detail record.
func ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortIt bool) (int, error) {
	return ForEachJSONInData("detail_", process, sortIt)
}

// LoadPDPProductIDs returns the set of product ids that already have a captured
// PDP response on disk, so pull_description can skip them (regardless of filename).
func LoadPDPProductIDs() map[string]struct{} {
	ids := map[string]struct{}{}

	dir := paths.PDPDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ids
	}

	for _, entry := range entries {
		if id, ok := pdpEntryID(dir, entry); ok {
			ids[id] = struct{}{}
		}
	}

	return ids
}

// PendingDescriptionCount reports how many detail records still need a PDP fetch
// (Description is JSON null), split by why they would otherwise be skipped: their
// id already has a captured PDP response, or the listing is tombstoned as
// no-longer-available. A present Description (including an empty string) counts as
// done, matching the pull_description loop.
func PendingDescriptionCount(existing, unavailable map[string]struct{}) (pending, alreadyFetched, unavailableCount int, err error) {
	_, err = ForEachDetail(func(_ string, jsonData map[string]any) bool {
		if jsonutil.GetKey(jsonData, "Description") != nil {
			return true
		}

		id, _ := jsonutil.GetKey(jsonData, "ID").(string)
		if id == "" {
			return true
		}
		if _, ok := unavailable[id]; ok {
			unavailableCount++
			return true
		}
		if _, ok := existing[id]; ok {
			alreadyFetched++
			return true
		}

		pending++
		return true
	}, false)
	return pending, alreadyFetched, unavailableCount, err
}

// DedupePDP removes duplicate PDP response files, keeping only the most recently
// modified file for each product id.
func DedupePDP() {
	dir := paths.PDPDir()
	byID, err := pdpFilesByID(dir)
	if err != nil {
		logging.LogError0("DedupePDP", "cannot read PDP dir", "dir", dir, "error", err)
		return
	}

	deleted := removeOlderPDPFiles(byID)

	logging.LogInfo0("DedupePDP", "finished", "uniqueIds", len(byID), "deleted", deleted)
}

// pdpFilesByID groups PDP response file paths by product id.
func pdpFilesByID(dir string) (map[string][]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	byID := map[string][]string{}
	for _, entry := range entries {
		if id, ok := pdpEntryID(dir, entry); ok {
			byID[id] = append(byID[id], filepath.Join(dir, entry.Name()))
		}
	}
	return byID, nil
}

// pdpEntryID returns the product id stored in a PDP response file.
func pdpEntryID(dir string, entry os.DirEntry) (string, bool) {
	if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
		return "", false
	}

	jsonData, err := readJSONObject(filepath.Join(dir, entry.Name()))
	if err != nil {
		return "", false
	}

	id, ok := jsonutil.GetKey(jsonData, "data.viewer.marketplace_product_details_page.target.id").(string)
	return id, ok && id != ""
}

// removeOlderPDPFiles deletes all but the newest file for each product id and
// returns how many files were removed.
func removeOlderPDPFiles(byID map[string][]string) int {
	deleted := 0
	for _, files := range byID {
		for _, path := range olderPDPFiles(files) {
			if err := os.Remove(path); err == nil {
				deleted++
			} else {
				logging.LogWarn0("DedupePDP", "could not remove", "path", path, "error", err)
			}
		}
	}
	return deleted
}

// olderPDPFiles returns the files to delete for one product id: everything but
// the most recently modified file.
func olderPDPFiles(files []string) []string {
	if len(files) < 2 {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		infoI, errI := os.Stat(files[i])
		infoJ, errJ := os.Stat(files[j])
		if errI != nil || errJ != nil {
			return files[i] > files[j]
		}
		return infoI.ModTime().After(infoJ.ModTime())
	})
	return files[1:]
}

// FillEmpty backfills the URL field on detail records that are missing it.
func FillEmpty() error {
	_, err := ForEachDetail(func(_ string, jsonData map[string]any) bool {
		productURL := jsonutil.GetKey(jsonData, "URL")
		if productURL != nil {
			return true
		}

		productIDStr, ok := jsonutil.GetKey(jsonData, "ID").(string)
		if !ok {
			return true
		}

		jsonData["URL"] = model.ProductIDToLink(productIDStr)

		store := NewProductFileStore(productIDStr)
		if _, err := store.Save(model.ToMarketplaceItemDetails(jsonData)); err != nil {
			logging.LogError0("FillEmpty", "error saving detail", "id", productIDStr, "error", err)
			return false
		}

		return true
	}, false)

	return err
}
