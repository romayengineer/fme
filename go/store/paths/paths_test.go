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

func TestResponseIDFromFileName(t *testing.T) {
	tests := []struct {
		name   string
		wantID string
		wantOK bool
	}{
		{name: "response_0_999.json", wantID: "999", wantOK: true},
		{name: filepath.Join("data", "Query", "response_0_123456789.json"), wantID: "123456789", wantOK: true},
		{name: "response_1700000000_000042.json"},
		{name: "detail_123.json"},
		{name: "response_0_.json"},
		{name: "response_0_999.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id, ok := ResponseIDFromFileName(test.name)
			if id != test.wantID || ok != test.wantOK {
				t.Errorf("ResponseIDFromFileName(%q) = %q,%v want %q,%v", test.name, id, ok, test.wantID, test.wantOK)
			}
		})
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
