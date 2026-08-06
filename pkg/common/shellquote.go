package common

import "strings"

// ShellQuote quotes s for safe embedding in a POSIX shell command line using
// single quotes, escaping embedded single quotes (FR-028 / export).
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
