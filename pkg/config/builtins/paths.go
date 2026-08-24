package builtins

import "path/filepath"

func joinPath(parts ...string) string { return filepath.Join(parts...) }
