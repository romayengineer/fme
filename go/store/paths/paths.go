// Package paths centralizes the on-disk data layout used by FME.
package paths

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// DataDir is the root of all scraped data.
	DataDir = "data"
	// DetailDirName holds the merged detail records.
	DetailDirName = "__Details"
	// PDPDirName holds captured MarketplacePDPContainerQuery responses.
	PDPDirName = "MarketplacePDPContainerQuery"
	// DebugDirName holds raw diagnostic payloads inside the PDP directory.
	DebugDirName = "debug"

	DetailPrefix   = "detail_"
	ResponsePrefix = "response"
)

// DetailDir is the directory of merged product detail records.
func DetailDir() string {
	return filepath.Join(DataDir, DetailDirName)
}

// PDPDir is the directory of captured PDP responses and state files.
func PDPDir() string {
	return filepath.Join(DataDir, PDPDirName)
}

// PDPDebugDir is the directory of raw diagnostic PDP payloads.
func PDPDebugDir() string {
	return filepath.Join(PDPDir(), DebugDirName)
}

// DetailFileName returns the deterministic filename for a product detail.
func DetailFileName(productID string) string {
	return fmt.Sprintf("%s%s.json", DetailPrefix, productID)
}

// DetailFilePath returns the full path for a product detail.
func DetailFilePath(productID string) string {
	return filepath.Join(DetailDir(), DetailFileName(productID))
}

// ResponseFile returns a random-order response path:
// <DataDir>/<friendlyName>/<prefix>_<timestamp>_<random>.json.
func ResponseFile(friendlyName, prefix string, timestamp int64, random int) string {
	name := fmt.Sprintf("%s_%d_%06d.json", prefix, timestamp, random)
	return filepath.Join(DataDir, friendlyName, name)
}

// ResponseFileByID returns a deterministic response path keyed by id. The
// numeric zero field keeps the <prefix>_<timestamp>_<name>.json shape that
// ParseTimestamp expects.
func ResponseFileByID(friendlyName, prefix, id string) string {
	name := fmt.Sprintf("%s_0_%s.json", prefix, id)
	return filepath.Join(DataDir, friendlyName, name)
}

// ParseTimestamp reads the numeric timestamp embedded in a response filename.
func ParseTimestamp(filePath string) (int64, error) {
	parts := strings.SplitN(filepath.Base(filePath), "_", 3)
	if len(parts) < 2 {
		return 0, fmt.Errorf("filename %q does not contain a timestamp", filepath.Base(filePath))
	}

	timestamp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse timestamp from %q: %w", filePath, err)
	}
	return timestamp, nil
}
