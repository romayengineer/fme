package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Rate-limit backoff bounds for the scheduler. A blocked account is retried
// from the minimum, doubling up to the maximum, and the backoff is reset after
// a successful request.
const (
	MinRateLimitBackoff = 15 * time.Minute
	MaxRateLimitBackoff = 2 * time.Hour
)

// ScheduleState is the durable bookkeeping the scheduler needs to resume after
// a restart instead of hammering a blocked account.
type ScheduleState struct {
	LastCycleAt     time.Time `json:"lastCycleAt,omitzero"`
	NextRunAt       time.Time `json:"nextRunAt,omitzero"`
	BlockedUntil    time.Time `json:"blockedUntil,omitzero"`
	LastSuccessAt   time.Time `json:"lastSuccessAt,omitzero"`
	LastThrottledAt time.Time `json:"lastThrottledAt,omitzero"`
	BackoffSeconds  int       `json:"backoffSeconds,omitzero"`
	LastTerm        string    `json:"lastTerm,omitempty"`
	LastError       string    `json:"lastError,omitempty"`
	Cycles          int       `json:"cycles,omitzero"`
	LastPulled      int       `json:"lastPulled,omitzero"`
	LastPending     int       `json:"lastPending,omitzero"`
}

// RateLimitBackoff returns the recorded backoff, or zero when there is none.
func (s *ScheduleState) RateLimitBackoff() time.Duration {
	return time.Duration(s.BackoffSeconds) * time.Second
}

// LoadScheduleState reads the state file. A missing file yields a zero state,
// which is the correct starting point for a fresh installation.
func LoadScheduleState(path string) (ScheduleState, error) {
	body, err := os.ReadFile(path) //nolint:gosec // path is user-configured input.
	if errors.Is(err, os.ErrNotExist) {
		return ScheduleState{}, nil
	}
	if err != nil {
		return ScheduleState{}, fmt.Errorf("read scheduler state %s: %w", path, err)
	}

	var state ScheduleState
	if err := json.Unmarshal(body, &state); err != nil {
		return ScheduleState{}, fmt.Errorf("parse scheduler state %s: %w", path, err)
	}
	return state, nil
}

// SaveScheduleState writes the state atomically so a crash cannot leave a
// partially written file behind.
func SaveScheduleState(path string, state ScheduleState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode scheduler state %s: %w", path, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".scheduler-state-*.json")
	if err != nil {
		return fmt.Errorf("create scheduler state %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := writeStateFile(tmp, body); err != nil {
		return fmt.Errorf("write scheduler state %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace scheduler state %s: %w", path, err)
	}
	return nil
}

// writeStateFile writes body, syncs it to disk, and closes the file.
func writeStateFile(file *os.File, body []byte) error {
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// NextRateLimitBackoff doubles the current backoff, capped at the maximum.
func NextRateLimitBackoff(current time.Duration) time.Duration {
	if current <= 0 {
		return MinRateLimitBackoff
	}
	if next := current * 2; next < MaxRateLimitBackoff {
		return next
	}
	return MaxRateLimitBackoff
}

// ResetRateLimitBackoff clears the backoff after a successful request.
func (s *ScheduleState) ResetRateLimitBackoff() {
	s.BackoffSeconds = 0
	s.BlockedUntil = time.Time{}
}
