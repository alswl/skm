package managers

import "strings"

// shellQuote quotes s for safe embedding in a POSIX shell command line using
// single quotes, escaping embedded single quotes (FR-028 / export).
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
