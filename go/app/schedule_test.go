package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// sleepRecorder records requested sleeps and optionally fails them.
type sleepRecorder struct {
	delays []time.Duration
	err    error
}

func (r *sleepRecorder) sleep(_ context.Context, d time.Duration) error {
	r.delays = append(r.delays, d)
	return r.err
}

// testScheduleOptions returns options rooted in a temporary directory.
func testScheduleOptions(t *testing.T, terms []string, now time.Time) ScheduleOptions {
	t.Helper()

	dir := t.TempDir()
	return ScheduleOptions{
		Terms:     terms,
		MaxCycles: 1,
		StatePath: dir + "/scheduler-state.json",
		LockPath:  dir + "/scheduler.lock",
		Now:       func() time.Time { return now },
	}
}

// testScheduleApp builds an app with a fake browser and detail store.
func testScheduleApp(t *testing.T) (*App, *fakePage, *fakeDetails) {
	t.Helper()

	page := &fakePage{}
	details := &fakeDetails{}
	app := New(Deps{
		Sessions: &fakeSessions{session: &Session{Context: &fakeContext{page: page}, Waiter: &fakeWaiter{}}},
		Details:  details,
		PDPState: &fakePDPState{},
		Sink:     &fakeResponseSink{},
	})
	return app, page, details
}

func TestRunScheduleSearchesTermsAndMaintains(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	app, page, details := testScheduleApp(t)
	sleeps := &sleepRecorder{}
	opts := testScheduleOptions(t, []string{"macbook m1", "iphone 16"}, now)
	opts.PerTermDelay = 10 * time.Minute
	opts.Interval = 6 * time.Hour
	opts.Sleep = sleeps.sleep

	if err := app.RunSchedule(context.Background(), opts); err != nil {
		t.Fatalf("RunSchedule: %v", err)
	}

	assertSearches(t, page, 2)
	assertCycleMaintenance(t, details)
	assertSleeps(t, sleeps, []time.Duration{10 * time.Minute})

	state := loadState(t, opts.StatePath)
	assertCycleState(t, state, now)
}

func TestRunScheduleBacksOffOnSearchThrottle(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	app, page, details := testScheduleApp(t)
	page.body = "This page isn't available right now"
	opts := testScheduleOptions(t, []string{"macbook m1"}, now)

	if err := app.RunSchedule(context.Background(), opts); err != nil {
		t.Fatalf("RunSchedule: %v", err)
	}

	assertInt(t, "DedupePDP calls", details.dedupeCalls, 0)
	state := loadState(t, opts.StatePath)
	assertInt(t, "BackoffSeconds", state.BackoffSeconds, int(MinRateLimitBackoff.Seconds()))
	assertBlockedState(t, state, now, MinRateLimitBackoff)
}

func TestRunScheduleHonorsExistingBlock(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	app, _, _ := testScheduleApp(t)
	sleeps := &sleepRecorder{}
	opts := testScheduleOptions(t, []string{"macbook m1"}, now)
	opts.Sleep = sleeps.sleep
	saveBlockedState(t, opts.StatePath, now, 5*time.Minute)

	if err := app.RunSchedule(context.Background(), opts); err != nil {
		t.Fatalf("RunSchedule: %v", err)
	}

	assertSleeps(t, sleeps, []time.Duration{5 * time.Minute})
	state := loadState(t, opts.StatePath)
	if state.BackoffSeconds != 0 || !state.BlockedUntil.IsZero() {
		t.Errorf("block was not reset after success: %+v", state)
	}
}

func TestRunScheduleRetriesSoonAfterSessionError(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	app := New(Deps{
		Sessions: &fakeSessions{err: errors.New("login failed")},
		Details:  &fakeDetails{},
		PDPState: &fakePDPState{},
		Sink:     &fakeResponseSink{},
	})
	opts := testScheduleOptions(t, []string{"macbook m1"}, now)

	if err := app.RunSchedule(context.Background(), opts); err != nil {
		t.Fatalf("RunSchedule: %v", err)
	}

	state := loadState(t, opts.StatePath)
	if state.LastError == "" {
		t.Error("LastError was not recorded")
	}
	if !state.LastSuccessAt.IsZero() {
		t.Errorf("LastSuccessAt = %v, want zero", state.LastSuccessAt)
	}
	if !state.NextRunAt.Equal(now.Add(MinRateLimitBackoff)) {
		t.Errorf("NextRunAt = %v, want %v", state.NextRunAt, now.Add(MinRateLimitBackoff))
	}
}

func TestRunScheduleRunNowIgnoresFutureNextRun(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	app, _, _ := testScheduleApp(t)
	sleeps := &sleepRecorder{}
	opts := testScheduleOptions(t, []string{"macbook m1"}, now)
	opts.Sleep = sleeps.sleep
	opts.RunNow = true
	if err := SaveScheduleState(opts.StatePath, ScheduleState{NextRunAt: now.Add(2 * time.Hour)}); err != nil {
		t.Fatalf("SaveScheduleState: %v", err)
	}

	if err := app.RunSchedule(context.Background(), opts); err != nil {
		t.Fatalf("RunSchedule: %v", err)
	}

	assertSleeps(t, sleeps, nil)
	assertInt(t, "Cycles", loadState(t, opts.StatePath).Cycles, 1)
}

func TestRunScheduleStopsCleanlyOnCancellation(t *testing.T) {
	app, _, _ := testScheduleApp(t)
	opts := testScheduleOptions(t, []string{"macbook m1"}, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := app.RunSchedule(ctx, opts); err != nil {
		t.Fatalf("RunSchedule: %v", err)
	}
	loadState(t, opts.StatePath)
}

func TestRunScheduleRequiresTerms(t *testing.T) {
	app, _, _ := testScheduleApp(t)

	if err := app.RunSchedule(context.Background(), ScheduleOptions{}); err == nil {
		t.Fatal("RunSchedule returned no error without terms")
	}
}

// assertSearches checks the number of Marketplace searches and their order.
func assertSearches(t *testing.T, page *fakePage, want int) {
	t.Helper()

	if len(page.gotos) != want {
		t.Fatalf("searches = %d, want %d (%v)", len(page.gotos), want, page.gotos)
	}
	if !strings.Contains(page.gotos[0], "query=macbook+m1") {
		t.Errorf("first search URL = %q", page.gotos[0])
	}
}

// assertCycleMaintenance checks the description pass, dedupe, and archive calls.
func assertCycleMaintenance(t *testing.T, details *fakeDetails) {
	t.Helper()

	assertInt(t, "DedupePDP calls", details.dedupeCalls, 1)
	assertInt(t, "ArchiveSearchResponses calls", details.archiveCall, 1)
	assertInt(t, "ProcessResponses calls", details.processed, 3)
}

// assertSleeps compares recorded sleep durations.
func assertSleeps(t *testing.T, sleeps *sleepRecorder, want []time.Duration) {
	t.Helper()

	if len(sleeps.delays) != len(want) {
		t.Fatalf("sleeps = %v, want %v", sleeps.delays, want)
	}
	for i := range want {
		if sleeps.delays[i] != want[i] {
			t.Errorf("sleep[%d] = %v, want %v", i, sleeps.delays[i], want[i])
		}
	}
}

// assertCycleState checks the persisted state after a successful cycle.
func assertCycleState(t *testing.T, state ScheduleState, now time.Time) {
	t.Helper()

	assertInt(t, "Cycles", state.Cycles, 1)
	if !state.NextRunAt.Equal(now.Add(DefaultScheduleInterval)) {
		t.Errorf("NextRunAt = %v, want %v", state.NextRunAt, now.Add(DefaultScheduleInterval))
	}
	if state.LastTerm != "iphone 16" || state.LastError != "" {
		t.Errorf("state = %+v", state)
	}
}

// assertBlockedState checks the recorded rate-limit backoff.
func assertBlockedState(t *testing.T, state ScheduleState, now time.Time, backoff time.Duration) {
	t.Helper()

	if !state.BlockedUntil.Equal(now.Add(backoff)) {
		t.Errorf("BlockedUntil = %v, want %v", state.BlockedUntil, now.Add(backoff))
	}
	if !state.LastThrottledAt.Equal(now) {
		t.Errorf("LastThrottledAt = %v, want %v", state.LastThrottledAt, now)
	}
}

// loadState reads the state file or fails the test.
func loadState(t *testing.T, path string) ScheduleState {
	t.Helper()

	state, err := LoadScheduleState(path)
	if err != nil {
		t.Fatalf("LoadScheduleState: %v", err)
	}
	return state
}

// saveBlockedState preloads a rate-limit block.
func saveBlockedState(t *testing.T, path string, now time.Time, backoff time.Duration) {
	t.Helper()

	state := ScheduleState{
		BackoffSeconds: int(backoff.Seconds()),
		BlockedUntil:   now.Add(backoff),
		NextRunAt:      now.Add(backoff),
	}
	if err := SaveScheduleState(path, state); err != nil {
		t.Fatalf("SaveScheduleState: %v", err)
	}
}
