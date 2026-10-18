package filestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnavailableMarkerRoundTrip(t *testing.T) {
	t.Chdir(t.TempDir())

	if ids := LoadUnavailableProductIds(); len(ids) != 0 {
		t.Fatalf("expected no unavailable ids, got %v", ids)
	}

	if err := MarkUnavailable("gone-1"); err != nil {
		t.Fatalf("MarkUnavailable: %v", err)
	}
	ids := LoadUnavailableProductIds()
	if _, ok := ids["gone-1"]; !ok || len(ids) != 1 {
		t.Fatalf("ids = %v, want {gone-1}", ids)
	}

	// Tombstones must not be mistaken for PDP responses.
	if pdp := LoadPDPProductIds(); len(pdp) != 0 {
		t.Errorf("LoadPDPProductIds = %v, want empty", pdp)
	}

	if err := ClearUnavailable("gone-1"); err != nil {
		t.Fatalf("ClearUnavailable: %v", err)
	}
	if ids := LoadUnavailableProductIds(); len(ids) != 0 {
		t.Errorf("ids after clear = %v, want empty", ids)
	}
}

func TestRecordPDPFailureThreshold(t *testing.T) {
	t.Chdir(t.TempDir())

	const threshold = 3
	for i := 1; i < threshold; i++ {
		became, count := RecordPDPFailure("product-x", threshold)
		if became {
			t.Fatalf("miss %d: became unavailable too early", i)
		}
		if count != i {
			t.Errorf("miss %d: count = %d, want %d", i, count, i)
		}
		if ids := LoadUnavailableProductIds(); len(ids) != 0 {
			t.Fatalf("miss %d: unexpectedly marked unavailable", i)
		}
	}

	became, count := RecordPDPFailure("product-x", threshold)
	if !became || count != threshold {
		t.Fatalf("final miss: became=%v count=%d, want true/%d", became, count, threshold)
	}
	if _, ok := LoadUnavailableProductIds()["product-x"]; !ok {
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
	path := filepath.Join(pdpDir(), debugDirName, "miss_abc.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("debug miss file missing: %v", err)
	}
}
