package main

import (
	"os"
	"testing"
)

func TestIsTerminalFileRejectsNonTerminals(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		t.Fatalf("stat temp file: %v", err)
	}

	if isTerminalFile(info, nil) {
		t.Error("regular file reported as a terminal")
	}
	if isTerminalFile(nil, nil) {
		t.Error("nil file info reported as a terminal")
	}
}
