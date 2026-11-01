package app

import (
	"errors"
	"testing"
)

// requireNoError fails the test on error.
func requireNoError(t *testing.T, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// assertInt compares one integer value.
func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %d, want %d", name, got, want)
	}
}

func TestMaintenanceDelegatesToDetailStore(t *testing.T) {
	details := &fakeDetails{
		stats: DescriptionStats{Total: 4, WithDescription: 2, Unavailable: 1, Empty: 1},
	}
	app := New(Deps{Details: details})

	requireNoError(t, app.ProcessResponses())
	assertInt(t, "ProcessResponses calls", details.processed, 1)

	requireNoError(t, app.FillEmpty())

	app.DedupePDP()
	assertInt(t, "DedupePDP calls", details.dedupeCalls, 1)

	stats, err := app.DescriptionStats()
	requireNoError(t, err)
	assertInt(t, "Total", stats.Total, 4)
	assertInt(t, "WithDescription", stats.WithDescription, 2)
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
