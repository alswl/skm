package main

import "os"

// main is the thin entry point: it only translates Execute's result into an
// exit code (go-cli-guides.md). os.Exit is never called elsewhere.
func main() {
	os.Exit(Execute())
}
