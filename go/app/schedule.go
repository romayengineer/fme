package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/romayengineer/fme/go/store/paths"
)

// Scheduler defaults. A cycle searches every term, then pulls every pending
// description before the next cycle starts.
const (
	// DefaultScheduleInterval is the pause between cycle starts.
	DefaultScheduleInterval = 6 * time.Hour
	// DefaultPerTermDelay is the pause between consecutive searches.
	DefaultPerTermDelay = 10 * time.Minute
	// DefaultScheduleScrollSeconds is how long a search keeps scrolling.
	DefaultScheduleScrollSeconds = 240
	// DefaultScheduleDelayMs is the pause between replayed description requests.
	DefaultScheduleDelayMs = 2000
)

// ScheduleOptions configures the unattended scheduler. Interval, paths, and the
// clock fall back to defaults; pacing values are used as-is (zero disables that
// pacing), because the CLI supplies them from flags.
type ScheduleOptions struct {
	Terms          []string
	Interval       time.Duration
	PerTermDelay   time.Duration
	ScrollSeconds  int
	DelayMs        int
	MaxCycles      int
	ArchiveOptions ArchiveOptions
	StatePath      string
	LockPath       string
	RefreshProfile func() error
	Now            func() time.Time
	Sleep          func(ctx context.Context, d time.Duration) error
}

// RunSchedule runs cycles until ctx is cancelled or MaxCycles is reached. A
// runtime error (browser, login, storage) is logged and retried on the next
// cycle instead of stopping the daemon; only invalid configuration or an
// unwritable state file returns an error.
func (a *App) RunSchedule(ctx context.Context, opts ScheduleOptions) error {
	opts = withScheduleDefaults(opts)
	if err := a.validateSchedule(opts); err != nil {
		return err
	}

	release, err := AcquireScheduleLock(opts.LockPath)
	if err != nil {
		return err
	}
	defer func() { _ = release() }()

	state, err := LoadScheduleState(opts.StatePath)
	if err != nil {
		return err
	}

	s := &scheduler{app: a, opts: opts, state: state}
	return s.run(ctx)
}

// withScheduleDefaults fills unset options.
func withScheduleDefaults(opts ScheduleOptions) ScheduleOptions {
	opts = withSchedulePaths(opts)
	opts = withScheduleClock(opts)
	if opts.Interval <= 0 {
		opts.Interval = DefaultScheduleInterval
	}
	return opts
}

// withSchedulePaths fills the state and lock paths.
func withSchedulePaths(opts ScheduleOptions) ScheduleOptions {
	if opts.StatePath == "" {
		opts.StatePath = paths.SchedulerStatePath()
	}
	if opts.LockPath == "" {
		opts.LockPath = paths.SchedulerLockPath()
	}
	return opts
}

// withScheduleClock fills the clock and sleep hooks.
func withScheduleClock(opts ScheduleOptions) ScheduleOptions {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Sleep == nil {
		opts.Sleep = SleepWithContext
	}
	return opts
}

// validateSchedule rejects a scheduler that cannot do useful work.
func (a *App) validateSchedule(opts ScheduleOptions) error {
	if a.Sessions == nil {
		return errors.New("schedule: session factory is not configured")
	}
	if a.Details == nil {
		return errors.New("schedule: detail store is not configured")
	}
	if len(opts.Terms) == 0 {
		return errors.New("schedule: no search terms configured")
	}
	return nil
}

// SleepWithContext sleeps for d, returning early when ctx is cancelled.
func SleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("scheduler sleep: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// scheduler holds the loop state for one RunSchedule call.
type scheduler struct {
	app   *App
	opts  ScheduleOptions
	state ScheduleState
}

// run executes cycles until the configured limit or a cancelled context.
func (s *scheduler) run(ctx context.Context) error {
	for cycles := 0; s.moreCycles(cycles); cycles++ {
		if err := s.runOne(ctx); err != nil {
			return s.finish(err)
		}
		if err := s.saveState(); err != nil {
			return err
		}
	}
	return nil
}

// moreCycles reports whether another cycle is allowed.
func (s *scheduler) moreCycles(done int) bool {
	return s.opts.MaxCycles == 0 || done < s.opts.MaxCycles
}

// runOne waits for its slot, runs a cycle, and records the completion.
func (s *scheduler) runOne(ctx context.Context) error {
	if err := s.waitForNext(ctx); err != nil {
		return err
	}
	if err := s.cycle(ctx); err != nil {
		return err
	}

	s.state.Cycles++
	s.state.LastCycleAt = s.now()
	return nil
}

// finish saves state on shutdown and turns cancellation into a clean exit.
func (s *scheduler) finish(err error) error {
	saveErr := s.saveState()

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	if saveErr != nil {
		return saveErr
	}
	return err
}

// waitForNext sleeps until the next run or the end of a rate-limit block.
func (s *scheduler) waitForNext(ctx context.Context) error {
	delay := s.nextTarget().Sub(s.now())
	if delay <= 0 {
		return nil
	}

	s.app.Logger.Info("schedule: waiting for next run", "seconds", int(delay.Seconds()))
	return s.opts.Sleep(ctx, delay)
}

// nextTarget returns the later of the next run and the rate-limit unblock time.
func (s *scheduler) nextTarget() time.Time {
	target := s.state.NextRunAt
	if s.state.BlockedUntil.After(target) {
		return s.state.BlockedUntil
	}
	return target
}

// cycle searches every term, pulls pending descriptions, and runs maintenance.
func (s *scheduler) cycle(ctx context.Context) error {
	s.refreshProfile()

	throttled, err := s.searchTerms(ctx)
	if err != nil {
		return err
	}
	if throttled {
		s.markRateLimited()
		return nil
	}
	return s.pullAndMaintain()
}

// searchTerms searches each configured term, stopping early on a throttle.
func (s *scheduler) searchTerms(ctx context.Context) (bool, error) {
	for index, term := range s.opts.Terms {
		throttled, err := s.searchTerm(ctx, index, term)
		if err != nil {
			return false, err
		}
		if throttled {
			return true, nil
		}
	}
	return false, nil
}

// searchTerm runs one search, recording a non-fatal error and moving on.
func (s *scheduler) searchTerm(ctx context.Context, index int, term string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.pauseBetweenTerms(ctx, index); err != nil {
		return false, err
	}

	s.state.LastTerm = term
	result, err := s.search(term)
	if err != nil {
		s.recordError(fmt.Errorf("search %q: %w", term, err))
		return false, nil
	}
	return result.Throttled, nil
}

// pauseBetweenTerms spaces consecutive searches, skipping the first.
func (s *scheduler) pauseBetweenTerms(ctx context.Context, index int) error {
	if index == 0 {
		return nil
	}
	return s.opts.Sleep(ctx, s.opts.PerTermDelay)
}

// search runs one Marketplace search without the interactive pause.
func (s *scheduler) search(term string) (SearchResult, error) {
	return s.app.Search(SearchOptions{
		Keywords:      term,
		ScrollDown:    true,
		ScrollSeconds: s.opts.ScrollSeconds,
	})
}

// pullAndMaintain pulls descriptions, then dedupes and archives responses.
func (s *scheduler) pullAndMaintain() error {
	result, err := s.app.PullDescriptions(PullDescriptionsOptions{DelayMs: s.opts.DelayMs})
	s.state.LastPending = result.Pending
	s.state.LastPulled = result.Pulled

	if err != nil {
		s.recordError(err)
		return nil
	}
	if result.RateLimited {
		s.markRateLimited()
		return nil
	}

	s.maintain()
	s.markSuccess()
	return nil
}

// maintain runs the post-cycle storage jobs.
func (s *scheduler) maintain() {
	s.app.DedupePDP()

	archived, pruned, err := s.app.ArchiveResponses(s.opts.ArchiveOptions)
	if err != nil {
		s.recordError(err)
		return
	}
	s.app.Logger.Info("schedule: maintenance finished", "archived", archived, "pruned", pruned)
}

// refreshProfile refreshes the copied Chrome profile when a hook is configured.
func (s *scheduler) refreshProfile() {
	if s.opts.RefreshProfile == nil {
		return
	}
	if err := s.opts.RefreshProfile(); err != nil {
		s.app.Logger.Warn("schedule: profile refresh failed", "error", err)
	}
}

// markRateLimited grows the backoff and schedules the retry.
func (s *scheduler) markRateLimited() {
	backoff := NextRateLimitBackoff(s.state.RateLimitBackoff())
	now := s.now()

	s.state.BackoffSeconds = int(backoff.Seconds())
	s.state.BlockedUntil = now.Add(backoff)
	s.state.NextRunAt = now.Add(backoff)
	s.state.LastThrottledAt = now
	s.state.LastError = "rate limited by Facebook"

	s.app.Logger.Warn("schedule: rate limited, backing off", "seconds", int(backoff.Seconds()))
}

// markSuccess resets the backoff and schedules the next cycle.
func (s *scheduler) markSuccess() {
	s.state.ResetRateLimitBackoff()
	s.state.LastSuccessAt = s.now()
	s.state.NextRunAt = s.now().Add(s.opts.Interval)
	s.state.LastError = ""
}

// recordError logs and stores a runtime error without stopping the loop.
func (s *scheduler) recordError(err error) {
	s.state.LastError = err.Error()
	s.app.Logger.Error("schedule: cycle error", "error", err)
}

// saveState persists the current state atomically.
func (s *scheduler) saveState() error {
	return SaveScheduleState(s.opts.StatePath, s.state)
}

// now returns the configured clock value.
func (s *scheduler) now() time.Time {
	return s.opts.Now()
}
