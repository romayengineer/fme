package pdpstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/romayengineer/fme/go/store/paths"
)

// requireNoUnavailableIDs fails when any tombstone is present.
func requireNoUnavailableIDs(t *testing.T) {
	t.Helper()

	if ids := LoadUnavailableProductIDs(); len(ids) != 0 {
		t.Fatalf("expected no unavailable ids, got %v", ids)
	}
}

func TestUnavailableMarkerRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())
	requireNoUnavailableIDs(t)

	if err := MarkUnavailable("gone-1"); err != nil {
		t.Fatalf("MarkUnavailable: %v", err)
	}
	ids := LoadUnavailableProductIDs()
	if _, ok := ids["gone-1"]; !ok || len(ids) != 1 {
		t.Fatalf("ids = %v, want {gone-1}", ids)
	}

	if err := ClearUnavailable("gone-1"); err != nil {
		t.Fatalf("ClearUnavailable: %v", err)
	}
	requireNoUnavailableIDs(t)
}

// assertMissesBelowThreshold records failures and checks the running count.
func assertMissesBelowThreshold(t *testing.T, threshold int) {
	t.Helper()

	for i := 1; i < threshold; i++ {
		became, count := RecordPDPFailure("product-x", threshold)
		if became {
			t.Fatalf("miss %d: became unavailable too early", i)
		}
		if count != i {
			t.Errorf("miss %d: count = %d, want %d", i, count, i)
		}
		requireNoUnavailableIDs(t)
	}
}

func TestRecordPDPFailureThreshold(t *testing.T) {
	t.Chdir(t.TempDir())

	const threshold = 3
	assertMissesBelowThreshold(t, threshold)

	became, count := RecordPDPFailure("product-x", threshold)
	if !became || count != threshold {
		t.Fatalf("final miss: became=%v count=%d, want true/%d", became, count, threshold)
	}
	if _, ok := LoadUnavailableProductIDs()["product-x"]; !ok {
		t.Error("product-x should be marked unavailable after threshold")
	}
	if _, err := os.Stat(filepath.Join(pdpDir(), missPrefix+"product-x.json")); !os.IsNotExist(err) {
		t.Error("miss counter should be removed once the tombstone is written")
	}
}

func TestWriteDebugMiss(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := WriteDebugMiss("abc", []byte(`{"data":{"viewer":{}}}`)); err != nil {
		t.Fatalf("WriteDebugMiss: %v", err)
	}
	path := filepath.Join(pdpDir(), paths.DebugDirName, "miss_abc.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("debug miss file missing: %v", err)
	}
}
