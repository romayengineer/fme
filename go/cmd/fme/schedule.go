package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/romayengineer/fme/go/app"
	"github.com/romayengineer/fme/go/store/paths"
)

// runSchedule runs the unattended scraper until the process is interrupted.
func runSchedule(a *app.App, flags Flags) error {
	terms, err := app.LoadTerms(flags.termsFile)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opts := app.ScheduleOptions{
		Terms:          terms,
		Interval:       flags.interval,
		PerTermDelay:   flags.perTermDelay,
		ScrollSeconds:  flags.scrollSeconds,
		DelayMs:        flags.scheduleDelayMs,
		ArchiveOptions: app.ArchiveOptions{OlderThan: flags.archiveAfter, KeepArchive: flags.keepArchived},
		StatePath:      paths.SchedulerStatePath(),
		LockPath:       paths.SchedulerLockPath(),
		RefreshProfile: refreshProfile,
	}
	if flags.once {
		opts.MaxCycles = 1
		opts.RunNow = true
	}
	return a.RunSchedule(ctx, opts)
}

// runStatus prints the persisted scheduler state.
func runStatus(_ *app.App, _ Flags) error {
	state, err := app.LoadScheduleState(paths.SchedulerStatePath())
	if err != nil {
		return err
	}
	printSchedulerState(state)
	return nil
}

// runArchiveResponses archives and prunes raw responses once.
func runArchiveResponses(a *app.App, flags Flags) error {
	archived, pruned, err := a.ArchiveResponses(app.ArchiveOptions{
		OlderThan:   flags.archiveAfter,
		KeepArchive: flags.keepArchived,
	})
	if err != nil {
		return err
	}
	fmt.Printf("archived %d, pruned %d\n", archived, pruned)
	return nil
}

// refreshProfile copies the logged-in Chrome profile for the next cycle.
func refreshProfile() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "scripts/sync-profile.sh", "--auto")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sync profile: %w", err)
	}
	return nil
}

// printSchedulerState renders the state file for `just status`.
func printSchedulerState(state app.ScheduleState) {
	fmt.Printf("cycles:              %d\n", state.Cycles)
	fmt.Printf("last cycle:          %s\n", formatWhen(state.LastCycleAt))
	fmt.Printf("next run:            %s\n", formatWhen(state.NextRunAt))
	fmt.Printf("blocked until:       %s\n", formatWhen(state.BlockedUntil))
	fmt.Printf("last success:        %s\n", formatWhen(state.LastSuccessAt))
	fmt.Printf("last throttled:      %s\n", formatWhen(state.LastThrottledAt))
	fmt.Printf("backoff:             %s\n", time.Duration(state.BackoffSeconds)*time.Second)
	fmt.Printf("last term:           %s\n", state.LastTerm)
	fmt.Printf("descriptions pulled: %d (pending %d)\n", state.LastPulled, state.LastPending)
	fmt.Printf("last error:          %s\n", state.LastError)
}

// formatWhen renders a timestamp, or "never" for the zero value.
func formatWhen(value time.Time) string {
	if value.IsZero() {
		return "never"
	}
	return value.Format(time.RFC3339)
}
