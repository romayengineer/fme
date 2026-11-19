package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/romayengineer/fme/go/store/paths"
)

// searchPayload builds a one-listing Marketplace search response.
func searchPayload(id string) map[string]any {
	return map[string]any{
		"data": map[string]any{
			"marketplace_search": map[string]any{
				"feed_units": map[string]any{
					"edges": []any{
						map[string]any{
							"node": map[string]any{
								"listing": map[string]any{
									"id":                        id,
									"marketplace_listing_title": "a listing",
								},
							},
						},
					},
				},
			},
		},
	}
}

// writeRawResponse writes a payload to a timestamped response file.
func writeRawResponse(t *testing.T, timestamp int64, payload any) string {
	t.Helper()

	path := paths.ResponseFile("CometMarketplaceSearchContentPaginationQuery", "response", timestamp, int(timestamp%1_000_000))
	writeResponseAt(t, path, payload)
	return path
}

// writeResponseAt writes a payload to path, creating its directory.
func writeResponseAt(t *testing.T, path string, payload any) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create response dir: %v", err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func TestMaterializeNewResponsesSavesOnlyFilesAfterSince(t *testing.T) {
	t.Chdir(t.TempDir())
	since := time.Now()
	oldPath := writeRawResponse(t, since.Add(-time.Hour).UnixNano(), searchPayload("old"))
	writeRawResponse(t, since.Add(time.Second).UnixNano(), searchPayload("new"))

	seen := map[string]struct{}{}
	ids, processed, err := MaterializeNewResponses(since, seen, 10)
	if err != nil {
		t.Fatalf("MaterializeNewResponses: %v", err)
	}
	assertProcessed(t, processed, 1)
	assertIDs(t, ids, []string{"new"})
	assertFilePresent(t, oldPath)
	assertFilePresent(t, paths.DetailFilePath("new"))
	assertFileMissing(t, paths.DetailFilePath("old"))

	ids, processed, err = MaterializeNewResponses(since, seen, 10)
	if err != nil {
		t.Fatalf("second MaterializeNewResponses: %v", err)
	}
	assertProcessed(t, processed, 0)
	assertIDs(t, ids, nil)
}

func TestMaterializeNewResponsesDeletesNoProductFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	since := time.Now()
	path := writeRawResponse(t, since.Add(time.Second).UnixNano(), map[string]any{"unrelated": true})

	ids, processed, err := MaterializeNewResponses(since, map[string]struct{}{}, 10)
	if err != nil {
		t.Fatalf("MaterializeNewResponses: %v", err)
	}
	assertProcessed(t, processed, 1)
	assertIDs(t, ids, nil)
	assertFileMissing(t, path)
}

func TestMaterializeNewResponsesHonorsLimit(t *testing.T) {
	t.Chdir(t.TempDir())
	since := time.Now()
	first := writeRawResponse(t, since.Add(time.Second).UnixNano(), searchPayload("first"))
	writeRawResponse(t, since.Add(2*time.Second).UnixNano(), searchPayload("second"))

	seen := map[string]struct{}{}
	ids, processed, err := MaterializeNewResponses(since, seen, 1)
	if err != nil {
		t.Fatalf("MaterializeNewResponses: %v", err)
	}
	assertProcessed(t, processed, 1)
	assertIDs(t, ids, []string{"first"})
	if _, ok := seen[first]; !ok {
		t.Errorf("processed file was not recorded in seen")
	}

	ids, processed, err = MaterializeNewResponses(since, seen, 1)
	if err != nil {
		t.Fatalf("second MaterializeNewResponses: %v", err)
	}
	assertProcessed(t, processed, 1)
	assertIDs(t, ids, []string{"second"})
}

func TestNewResponsePathsSkipPDPDir(t *testing.T) {
	t.Chdir(t.TempDir())
	searchPath := writeRawResponse(t, time.Now().UnixNano(), searchPayload("search"))
	pdpPath := paths.ResponseFile(paths.PDPDirName, "response", time.Now().UnixNano(), 7)
	writeResponseAt(t, pdpPath, searchPayload("pdp"))

	got, err := getNewResponsePaths()
	if err != nil {
		t.Fatalf("getNewResponsePaths: %v", err)
	}
	assertStrings(t, got, []string{searchPath})
}

// assertProcessed checks how many response files a call processed.
func assertProcessed(t *testing.T, got, want int) {
	t.Helper()

	if got != want {
		t.Errorf("processed = %d, want %d", got, want)
	}
}

// assertIDs checks the saved product ids in order.
func assertIDs(t *testing.T, got, want []string) {
	t.Helper()

	assertStrings(t, got, want)
}

// assertStrings checks two string slices for equality in order.
func assertStrings(t *testing.T, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("item[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
