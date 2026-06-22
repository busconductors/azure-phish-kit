package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func main() {
	// Switch stdin to raw mode so we receive keystrokes individually.
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tui: cannot enter raw mode: %v\n", err)
		os.Exit(1)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	app := NewApp()
	app.Run()
}
