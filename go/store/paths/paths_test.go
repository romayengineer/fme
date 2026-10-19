package paths

import (
	"path/filepath"
	"testing"
)

func TestDetailPaths(t *testing.T) {
	if got := DetailDir(); got != filepath.Join("data", "__Details") {
		t.Errorf("DetailDir = %q", got)
	}
	if got := DetailFileName("123"); got != "detail_123.json" {
		t.Errorf("DetailFileName = %q", got)
	}
	if got := DetailFilePath("123"); got != filepath.Join("data", "__Details", "detail_123.json") {
		t.Errorf("DetailFilePath = %q", got)
	}
}

func TestResponsePaths(t *testing.T) {
	got := ResponseFile("Query", ResponsePrefix, 1700000000, 42)
	want := filepath.Join("data", "Query", "response_1700000000_000042.json")
	if got != want {
		t.Errorf("ResponseFile = %q, want %q", got, want)
	}

	got = ResponseFileByID("Query", ResponsePrefix, "999")
	want = filepath.Join("data", "Query", "response_0_999.json")
	if got != want {
		t.Errorf("ResponseFileByID = %q, want %q", got, want)
	}
}

func TestParseTimestamp(t *testing.T) {
	timestamp, err := ParseTimestamp(filepath.Join("data", "Query", "response_1700000000_000042.json"))
	if err != nil {
		t.Fatalf("ParseTimestamp: %v", err)
	}
	if timestamp != 1700000000 {
		t.Errorf("timestamp = %d", timestamp)
	}

	if _, err := ParseTimestamp("response.json"); err == nil {
		t.Error("expected an error for a filename without a timestamp")
	}
	if _, err := ParseTimestamp("response_not-a-number_file.json"); err == nil {
		t.Error("expected an error for a non-numeric timestamp")
	}
}
