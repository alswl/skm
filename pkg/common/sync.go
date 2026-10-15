package common

// SyncRecord travels with the repository; transient attempts never live here.
type SyncRecord struct {
	Schema          int    `json:"schema_version"`
	EntryPath       string `json:"entry_path"`
	Name            string `json:"name"`
	Generation      uint64 `json:"generation"`
	Origin          Origin `json:"origin"`
	Active          string `json:"active"`
	ForkPath        string `json:"fork_path,omitempty"`
	UpstreamVersion string `json:"upstream_version"`
	ForkVersion     string `json:"fork_version,omitempty"`
	UpstreamDigest  string `json:"upstream_digest"`
	ForkDigest      string `json:"fork_digest,omitempty"`
}

// SyncView is a read-only catalog summary populated off the TUI event loop.
type SyncView struct {
	Mode, ForkState, ForkPath, UpstreamPath, BaselineVersion, Customization, AttemptPath string
	RollbackAvailable                                                                    bool
}
