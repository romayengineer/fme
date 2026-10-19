package app

import (
	"errors"
	"testing"
)

func TestMaintenanceDelegatesToDetailStore(t *testing.T) {
	details := &fakeDetails{
		stats: DescriptionStats{Total: 4, WithDescription: 2, Unavailable: 1, Empty: 1},
	}
	app := New(Deps{Details: details})

	if err := app.ProcessResponses(); err != nil {
		t.Fatalf("ProcessResponses: %v", err)
	}
	if details.processed != 1 {
		t.Errorf("ProcessResponses called %d times, want 1", details.processed)
	}

	if err := app.FillEmpty(); err != nil {
		t.Fatalf("FillEmpty: %v", err)
	}

	app.DedupePDP()
	if details.dedupeCalls != 1 {
		t.Errorf("DedupePDP called %d times, want 1", details.dedupeCalls)
	}

	stats, err := app.DescriptionStats()
	if err != nil {
		t.Fatalf("DescriptionStats: %v", err)
	}
	if stats.Total != 4 || stats.WithDescription != 2 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestMaintenanceReturnsDetailErrors(t *testing.T) {
	app := New(Deps{Details: &fakeDetails{err: errors.New("read failed")}})

	if err := app.ProcessResponses(); err == nil {
		t.Fatal("ProcessResponses returned no error")
	}
}

func TestMaintenanceRequiresDetailStore(t *testing.T) {
	app := New(Deps{})

	if err := app.ProcessResponses(); err == nil {
		t.Fatal("ProcessResponses returned no error without a detail store")
	}
	if err := app.FillEmpty(); err == nil {
		t.Fatal("FillEmpty returned no error without a detail store")
	}
	if _, err := app.DescriptionStats(); err == nil {
		t.Fatal("DescriptionStats returned no error without a detail store")
	}
}
