// Package logging provides the project's structured logging helpers.
//
// It wraps log/slog with fixed severity levels and a funcName attribute so the
// call site is always part of the log record.
package logging

import (
	"context"
	"log/slog"
)

// Severity levels. LD0 is debug, LI0 is informational, LW0 is warning, and
// LE0 is error.
const (
	LD0 slog.Level = -4
	LI0 slog.Level = 0
	LW0 slog.Level = 4
	LE0 slog.Level = 8
)

// Log records msg at level and adds funcName as the first attribute.
func Log(level slog.Level, funcName string, msg string, args ...any) {
	allArgs := append([]any{"funcName", funcName}, args...)
	slog.Log(context.Background(), level, msg, allArgs...)
}

// LogDebug0 logs a debug message at the most verbose level.
func LogDebug0(funcName string, msg string, args ...any) {
	Log(LD0, funcName, msg, args...)
}

// LogInfo0 logs an informational message at level 0.
func LogInfo0(funcName string, msg string, args ...any) {
	Log(LI0, funcName, msg, args...)
}

// LogWarn0 logs a warning message.
func LogWarn0(funcName string, msg string, args ...any) {
	Log(LW0, funcName, msg, args...)
}

// LogError0 logs an error message.
func LogError0(funcName string, msg string, args ...any) {
	Log(LE0, funcName, msg, args...)
}
