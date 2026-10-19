package filestore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/romayengineer/fme/go/core/jsonutil"
	"github.com/romayengineer/fme/go/core/logging"
	"github.com/romayengineer/fme/go/marketplace/extract"
	"github.com/romayengineer/fme/go/marketplace/model"
	"github.com/romayengineer/fme/go/store/paths"
)

func SaveProductsIfAny(products []model.MarketplaceItemDetails) bool {
	if len(products) == 0 {
		return false
	}

	for _, product := range products {
		productID, ok := product.ID.(string)
		if !ok || productID == "" {
			logging.LogWarn0("SaveProductsIfAny", "skipping product without a valid id", "id", product.ID)
			continue
		}

		store := NewProductFileStore(productID)
		if _, err := store.Save(product); err != nil {
			logging.LogError0("SaveProductsIfAny", "error saving product", "id", productID, "error", err)
		}
	}

	return true
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

func ProcessData(StartTimeToProcess int64) (int64, error) {
	var lastTimestamp int64
	var lastFilePath string
	var filesProcessedCounter int
	var filesDeletedCounter int
	var gotRateLimit bool

	productExtractors := extract.NewProductExtractors()
	filesReadCounter, err := ForEachResponse(func(filePath string, jsonData map[string]any) bool {
		lastFilePath = filePath

		fileTimestamp, err := GetTimestamp(filePath)
		if err != nil {
			return true
		}

		if fileTimestamp < StartTimeToProcess {
			return true
		}

		filesProcessedCounter += 1

		if hasAny := SaveProductsIfAny(productExtractors.ExtractAll(jsonData)); hasAny {
			return true
		}

		if extract.IsErrorRateLimit(jsonData) {
			gotRateLimit = true
		}

		logging.LogDebug0("ProcessData", "no product found, deleting file", "path", filePath)
		if err := os.Remove(filePath); err != nil {
			logging.LogError0("ProcessData", "error deleting file", "path", filePath, "error", err)
		}

		filesDeletedCounter += 1

		return true

	}, true)

	lastTimestamp, _ = GetTimestamp(lastFilePath)

	logging.LogInfo0("ProcessData", "all files processed", "lastTimestamp", lastTimestamp, "filesProcessedCounter", filesProcessedCounter, "filesReadCounter", filesReadCounter, "filesDeletedCounter", filesDeletedCounter)

	if gotRateLimit {
		logging.LogWarn0("ProcessData", "got rate limit")
	}

	return lastTimestamp, err
}

func GetTimestamp(filePath string) (int64, error) {
	timestamp, err := paths.ParseTimestamp(filePath)
	if err != nil {
		logging.LogWarn0("GetTimestamp", "could not read timestamp", "filePath", filePath, "error", err)
		return 0, err
	}
	return timestamp, nil
}

func WriteJsonResponse(jsonDatas []any, friendly_name string) (int, error) {
	jsonCounter := 0
	var err error
	for _, jsonData := range jsonDatas {
		if err = WriteRandomJsonFileIndented("response", friendly_name, jsonData); err != nil {
			return jsonCounter, err
		}
		jsonCounter += 1
	}

	return jsonCounter, nil
}

// WriteJsonResponseById writes response JSON to a deterministic filename keyed by id,
// overwriting any previous file for the same id. This makes writes idempotent so
// re-running never accumulates duplicate responses for the same product.
//
// The name keeps the "<prefix>_<number>_<...>.json" shape because GetTimestamp parses
// the second underscore-separated field as the (numeric) timestamp.
func WriteJsonResponseById(jsonDatas []any, friendlyName string, id string) (int, error) {
	counter := 0
	for _, jsonData := range jsonDatas {
		if err := WriteJsonFileDeterministic("response", friendlyName, id, jsonData); err != nil {
			return counter, err
		}
		counter++
	}
	return counter, nil
}

func WriteJsonFileDeterministic(prefix string, friendlyName string, key string, jsonData any) error {
	indented, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		return err
	}
	filename := paths.ResponseFileByID(friendlyName, prefix, key)
	return WriteFileAndDirs(filename, indented, 0644)
}

func WriteFileAndDirs(name string, data []byte, perm os.FileMode) error {
	nameDir := filepath.Dir(name)
	if err := os.MkdirAll(nameDir, 0755); err != nil {
		return err
	}
	return writeFileAtomic(name, data, perm)
}

// writeFileAtomic writes data to a temporary file in the destination directory
// and renames it into place, so readers never observe a partially written file
// and a crash leaves the previous version intact.
func writeFileAtomic(name string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(name)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(name)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", name, err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file for %s: %w", name, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file for %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file for %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file for %s: %w", name, err)
	}
	if err := os.Rename(tmpName, name); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp file to %s: %w", name, err)
	}

	return nil
}

func TimeNow() int64 {
	return time.Now().UnixNano()
}

func TimeYesterday() int64 {
	return time.Now().AddDate(0, 0, -1).UnixNano()
}

func WriteRandomJsonFile(prefix string, friendly_name string, body []byte) error {
	timestamp := TimeNow()
	random := rand.Intn(1000000)

	filename := paths.ResponseFile(friendly_name, prefix, timestamp, random)

	return WriteFileAndDirs(filename, body, 0644)
}

func WriteRandomJsonFileIndented(prefix string, friendly_name string, jsonData any) error {
	indented, _ := json.MarshalIndent(jsonData, "", "  ")
	return WriteRandomJsonFile(prefix, friendly_name, indented)
}

func GetFilePaths(prefix string, sortit bool) ([]string, error) {

	filePaths := []string{}

	err := filepath.WalkDir(DataDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		// skip archived folder
		if strings.Contains(path, "archived") {
			return nil
		}

		fileName := d.Name()

		if !strings.HasPrefix(fileName, prefix) || !strings.HasSuffix(fileName, ".json") {
			return nil
		}

		filePaths = append(filePaths, path)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", DataDir, err)
	}

	if sortit {
		// if sort is true sort filePaths in ascendant order
		sort.Slice(filePaths, func(i, j int) bool {
			return filepath.Base(filePaths[i]) < filepath.Base(filePaths[j])
		})
	} else {
		// else sort filePaths in random order
		rand.Shuffle(len(filePaths), func(i, j int) { filePaths[i], filePaths[j] = filePaths[j], filePaths[i] })
	}

	return filePaths, nil
}

func ForEachJsonInData(prefix string, process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error) {
	// open and read all files in data folder that start with response and end in .json

	filesReadCounter := 0
	var errs []error

	filePaths, err := GetFilePaths(prefix, sortit)
	if err != nil {
		return 0, err
	}

	for _, filePath := range filePaths {
		body, err := os.ReadFile(filePath)
		if err != nil {
			logging.LogError0("ForEachJsonInData", "error reading file", "path", filePath, "error", err)
			errs = append(errs, fmt.Errorf("read %s: %w", filePath, err))
			continue
		}

		var jsonData map[string]any
		if err := json.Unmarshal(body, &jsonData); err != nil {
			logging.LogError0("ForEachJsonInData", "error parsing JSON", "path", filePath, "error", err)
			errs = append(errs, fmt.Errorf("parse %s: %w", filePath, err))
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

func ForEachResponse(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error) {
	return ForEachJsonInData("response_", process, sortit)
}

func ForEachDetail(process func(filePath string, jsonData map[string]any) bool, sortit bool) (int, error) {
	return ForEachJsonInData("detail_", process, sortit)
}

// LoadPDPProductIds returns the set of product ids that already have a captured PDP
// response on disk, so pull_description can skip them (regardless of filename).
func LoadPDPProductIds() map[string]struct{} {
	ids := map[string]struct{}{}

	dir := paths.PDPDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ids
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var jsonData map[string]any
		if err := json.Unmarshal(body, &jsonData); err != nil {
			continue
		}

		id := jsonutil.GetKey(jsonData, "data.viewer.marketplace_product_details_page.target.id")
		if idStr, ok := id.(string); ok {
			ids[idStr] = struct{}{}
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		logging.LogError0("DedupePDP", "cannot read PDP dir", "dir", dir, "error", err)
		return
	}

	byID := map[string][]string{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var jsonData map[string]any
		if err := json.Unmarshal(body, &jsonData); err != nil {
			continue
		}
		id := jsonutil.GetKey(jsonData, "data.viewer.marketplace_product_details_page.target.id")
		idStr, ok := id.(string)
		if !ok {
			continue
		}
		byID[idStr] = append(byID[idStr], path)
	}

	deleted := 0
	for _, paths := range byID {
		if len(paths) < 2 {
			continue
		}
		sort.Slice(paths, func(i, j int) bool {
			infoI, errI := os.Stat(paths[i])
			infoJ, errJ := os.Stat(paths[j])
			if errI != nil || errJ != nil {
				return paths[i] > paths[j]
			}
			return infoI.ModTime().After(infoJ.ModTime())
		})
		for _, path := range paths[1:] {
			if err := os.Remove(path); err == nil {
				deleted++
			} else {
				logging.LogWarn0("DedupePDP", "could not remove", "path", path, "error", err)
			}
		}
	}

	logging.LogInfo0("DedupePDP", "finished", "uniqueIds", len(byID), "deleted", deleted)
}

func FillEmpty() error {
	_, err := ForEachDetail(func(filePath string, jsonData map[string]any) bool {
		productUrl := jsonutil.GetKey(jsonData, "URL")
		if productUrl != nil {
			return true
		}

		productId := jsonutil.GetKey(jsonData, "ID")
		if productId == nil {
			return true
		}

		productIdStr, ok := productId.(string)

		if !ok {
			return true
		}

		productUrl = model.ProductIDTolink(productIdStr)

		jsonData["URL"] = productUrl

		store := NewProductFileStore(productIdStr)
		if _, err := store.Save(model.ToMarketplaceItemDetails(jsonData)); err != nil {
			logging.LogError0("FillEmpty", "error saving detail", "id", productIdStr, "error", err)
			return false
		}

		return true
	}, false)

	return err
}
