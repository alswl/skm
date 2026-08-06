package common

import (
	"encoding/json"
	"io"
)

// PrintJSON encodes v as a single JSON object to w (stdout) followed by a
// newline. Struct field order is preserved, which keeps golden fixtures
// deterministic (SC-002). It must be the only writer to stdout for a command.
func PrintJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
