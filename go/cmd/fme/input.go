package main

import (
	"bufio"
	"fmt"
	"os"
)

// waitForInput pauses a browser action until the user presses Enter, so they can
// inspect the page or finish a manual challenge.
func waitForInput() {
	fmt.Print("waiting for input: ")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}
