package providers

// Capability describes what a Provider handles, shown by `provider list`
// without fetching anything (002-open-provider-target FR-002). A plugin that
// doesn't implement the `capability` action falls back to {ID,Label,"",nil}.
type Capability struct {
	ID          string
	Label       string
	Description string
	Schemes     []string
	Icon        string // optional one-glyph marker shown next to the provider's entries in the TUI; "" means none
}

// ProviderError is a diagnosable error from a Provider operation: a stable
// code plus a human message naming the address/reason (FR-003). Code is one
// of the constants below; unrecognized codes from legacy plugin responses
// (bare error strings) map to CodeFetchFailed.
type ProviderError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ProviderError) Error() string { return e.Message }

const (
	CodeUnsupportedAddress = "unsupported_address"
	CodeNormalizeFailed    = "normalize_failed"
	CodeFetchFailed        = "fetch_failed"
	CodeProtocolError      = "protocol_error"
	CodeTimeout            = "timeout"
	CodeDuplicateID        = "duplicate_id"
	CodeEmptyID            = "empty_id"
)

// ProviderLoadFailure records why a plugin failed to load during discovery
// (FR-006). Retaining these — instead of only logging — lets `provider
// list`/`validate` show the specific reason without breaking isolation
// (FR-007): a failure here is never fatal to skm or to other providers.
type ProviderLoadFailure struct {
	Path   string
	ID     string // best-effort; may be empty if the plugin never returned one
	Reason ProviderError
}
