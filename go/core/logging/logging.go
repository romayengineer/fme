// Package logging provides the project's structured logging helpers.
//
// It wraps log/slog with fixed severity levels and a funcName attribute so the
// call site is always part of the log record.
package logging

import (
	"context"
	"log/slog"
)

// Severity levels. LD0-LD3 are debug (most to least verbose), LI0-LI3 are
// informational, LW0 is warning, and LE0 is error.
const (
	LD0 slog.Level = -4
	LD1 slog.Level = -3
	LD2 slog.Level = -2
	LD3 slog.Level = -1
	LI0 slog.Level = 0
	LI1 slog.Level = 1
	LI2 slog.Level = 2
	LI3 slog.Level = 3
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

// LogDebug1 logs a debug message at level 1.
func LogDebug1(funcName string, msg string, args ...any) {
	Log(LD1, funcName, msg, args...)
}

// LogDebug2 logs a debug message at level 2.
func LogDebug2(funcName string, msg string, args ...any) {
	Log(LD2, funcName, msg, args...)
}

// LogDebug3 logs a debug message at the least verbose debug level.
func LogDebug3(funcName string, msg string, args ...any) {
	Log(LD3, funcName, msg, args...)
}

// LogInfo0 logs an informational message at level 0.
func LogInfo0(funcName string, msg string, args ...any) {
	Log(LI0, funcName, msg, args...)
}

// LogInfo1 logs an informational message at level 1.
func LogInfo1(funcName string, msg string, args ...any) {
	Log(LI1, funcName, msg, args...)
}

// LogInfo2 logs an informational message at level 2.
func LogInfo2(funcName string, msg string, args ...any) {
	Log(LI2, funcName, msg, args...)
}

// LogInfo3 logs an informational message at level 3.
func LogInfo3(funcName string, msg string, args ...any) {
	Log(LI3, funcName, msg, args...)
}

// LogWarn0 logs a warning message.
func LogWarn0(funcName string, msg string, args ...any) {
	Log(LW0, funcName, msg, args...)
}

// LogError0 logs an error message.
func LogError0(funcName string, msg string, args ...any) {
	Log(LE0, funcName, msg, args...)
}
