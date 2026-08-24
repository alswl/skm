package providers

import (
	"os"
	"path/filepath"
	"strings"
)

func normalizeLocalSource(source string) string {
	source = strings.TrimSpace(source)
	if source != "~" && !strings.HasPrefix(source, "~/") {
		return source
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return source
	}
	if source == "~" {
		return home
	}
	return filepath.Join(home, source[2:])
}
