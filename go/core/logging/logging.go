package logging

import (
	"context"
	"log/slog"
)

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

func Log(level slog.Level, funcName string, msg string, args ...any) {
	allArgs := append([]any{"funcName", funcName}, args...)
	slog.Log(context.Background(), level, msg, allArgs...)
}

func LogDebug0(funcName string, msg string, args ...any) {
	Log(LD0, funcName, msg, args...)
}

func LogDebug1(funcName string, msg string, args ...any) {
	Log(LD1, funcName, msg, args...)
}

func LogDebug2(funcName string, msg string, args ...any) {
	Log(LD2, funcName, msg, args...)
}

func LogDebug3(funcName string, msg string, args ...any) {
	Log(LD3, funcName, msg, args...)
}

func LogInfo0(funcName string, msg string, args ...any) {
	Log(LI0, funcName, msg, args...)
}

func LogInfo1(funcName string, msg string, args ...any) {
	Log(LI1, funcName, msg, args...)
}

func LogInfo2(funcName string, msg string, args ...any) {
	Log(LI2, funcName, msg, args...)
}

func LogInfo3(funcName string, msg string, args ...any) {
	Log(LI3, funcName, msg, args...)
}

func LogWarn0(funcName string, msg string, args ...any) {
	Log(LW0, funcName, msg, args...)
}

func LogError0(funcName string, msg string, args ...any) {
	Log(LE0, funcName, msg, args...)
}
