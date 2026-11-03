package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScheduleStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler-state.json")
	want := ScheduleState{
		LastCycleAt:    time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC),
		NextRunAt:      time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC),
		BlockedUntil:   time.Date(2026, 11, 2, 12, 30, 0, 0, time.UTC),
		LastSuccessAt:  time.Date(2026, 11, 2, 11, 55, 0, 0, time.UTC),
		BackoffSeconds: 900,
		LastTerm:       "macbook m1",
		LastError:      "rate limited",
		Cycles:         3,
	}

	if err := SaveScheduleState(path, want); err != nil {
		t.Fatalf("SaveScheduleState: %v", err)
	}

	got, err := LoadScheduleState(path)
	if err != nil {
		t.Fatalf("LoadScheduleState: %v", err)
	}
	assertScheduleState(t, got, want)
}

// assertScheduleState compares the fields that must survive a round trip.
func assertScheduleState(t *testing.T, got, want ScheduleState) {
	t.Helper()

	if !got.LastCycleAt.Equal(want.LastCycleAt) {
		t.Errorf("LastCycleAt = %v, want %v", got.LastCycleAt, want.LastCycleAt)
	}
	if got.BackoffSeconds != want.BackoffSeconds {
		t.Errorf("BackoffSeconds = %d, want %d", got.BackoffSeconds, want.BackoffSeconds)
	}
	if got.Cycles != want.Cycles {
		t.Errorf("Cycles = %d, want %d", got.Cycles, want.Cycles)
	}
}

func TestLoadScheduleStateMissingFile(t *testing.T) {
	state, err := LoadScheduleState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadScheduleState: %v", err)
	}
	if !state.LastCycleAt.IsZero() || state.Cycles != 0 {
		t.Errorf("state = %+v, want zero", state)
	}
}

func TestLoadScheduleStateCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	if _, err := LoadScheduleState(path); err == nil {
		t.Error("LoadScheduleState returned no error for corrupt JSON")
	}
}

func TestNextRateLimitBackoff(t *testing.T) {
	cases := []struct {
		name    string
		current time.Duration
		want    time.Duration
	}{
		{name: "first block", current: 0, want: MinRateLimitBackoff},
		{name: "double", current: MinRateLimitBackoff, want: 2 * MinRateLimitBackoff},
		{name: "cap", current: MaxRateLimitBackoff, want: MaxRateLimitBackoff},
		{name: "above cap", current: MaxRateLimitBackoff * 2, want: MaxRateLimitBackoff},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NextRateLimitBackoff(tc.current); got != tc.want {
				t.Errorf("NextRateLimitBackoff(%v) = %v, want %v", tc.current, got, tc.want)
			}
		})
	}
}

func TestScheduleStateResetBackoff(t *testing.T) {
	state := ScheduleState{BackoffSeconds: 900, BlockedUntil: time.Now()}
	state.ResetRateLimitBackoff()

	if state.BackoffSeconds != 0 || !state.BlockedUntil.IsZero() {
		t.Errorf("state after reset = %+v", state)
	}
}

func TestAcquireScheduleLockIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler.lock")

	release := mustAcquireScheduleLock(t, path)
	assertScheduleLockHeld(t, path)
	mustReleaseScheduleLock(t, release)

	releaseAgain := mustAcquireScheduleLock(t, path)
	mustReleaseScheduleLock(t, releaseAgain)
}

// mustAcquireScheduleLock acquires the lock or fails the test.
func mustAcquireScheduleLock(t *testing.T, path string) func() error {
	t.Helper()

	release, err := AcquireScheduleLock(path)
	if err != nil {
		t.Fatalf("AcquireScheduleLock: %v", err)
	}
	return release
}

// assertScheduleLockHeld fails when a second acquisition succeeds.
func assertScheduleLockHeld(t *testing.T, path string) {
	t.Helper()

	if _, err := AcquireScheduleLock(path); err == nil {
		t.Fatal("second AcquireScheduleLock succeeded, want failure")
	}
}

// mustReleaseScheduleLock releases the lock or fails the test.
func mustReleaseScheduleLock(t *testing.T, release func() error) {
	t.Helper()

	if err := release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}
