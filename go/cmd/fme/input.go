package main

import (
	"bufio"
	"fmt"
	"os"
)

// waitForInput pauses a browser action until the user presses Enter, so they can
// inspect the page or finish a manual challenge. Piped or redirected stdin
// (cron, CI) is not a terminal, so unattended runs continue without blocking.
func waitForInput() {
	if !stdinIsTerminal() {
		return
	}

	fmt.Print("waiting for input: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}

// stdinIsTerminal reports whether stdin is attached to a terminal.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return isTerminalFile(info, err)
}

// isTerminalFile reports whether a file description is a character device.
func isTerminalFile(info os.FileInfo, err error) bool {
	return err == nil && info != nil && info.Mode()&os.ModeCharDevice != 0
}
