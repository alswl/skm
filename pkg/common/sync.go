package common

// BaselineRef identifies the immutable raw tree used for a comparison. Git
// references are reconstructed through a verified full commit; snapshots are
// versioned content and remain valid when the original source disappears.
type BaselineRef struct {
	Kind           string `json:"kind"` // git or snapshot
	Revision       string `json:"revision"`
	RawDigest      string `json:"raw_digest"`
	SnapshotDigest string `json:"snapshot_digest,omitempty"`
}

// SyncOperation is persisted only while a v2 operation is unfinished. Its
// fixed inputs make status/rollback independent of a later provider fetch.
type SyncOperation struct {
	ID             string `json:"id,omitempty"`
	Kind           string `json:"kind,omitempty"`
	State          string `json:"state,omitempty"`
	BackupComplete bool   `json:"backup_complete,omitempty"`
}

// SyncRecord travels with the repository; transient attempts never live here.
type SyncRecord struct {
	Schema          int          `json:"schema_version"`
	EntryPath       string       `json:"entry_path"`
	Name            string       `json:"name"`
	Generation      uint64       `json:"generation"`
	Origin          Origin       `json:"origin"`
	Active          string       `json:"active"`
	ForkPath        string       `json:"fork_path,omitempty"`
	UpstreamVersion string       `json:"upstream_version"`
	ForkVersion     string       `json:"fork_version,omitempty"`
	UpstreamDigest  string       `json:"upstream_digest"`
	ForkDigest      string       `json:"fork_digest,omitempty"`
	UpstreamBase    BaselineRef  `json:"upstream_base,omitempty"`
	ForkBase        *BaselineRef `json:"fork_base,omitempty"`
	// Baseline digests describe raw upstream trees. Accepted content digests
	// describe the installed, possibly normalised trees used for edit checks.
	UpstreamContentDigest string         `json:"upstream_content_digest,omitempty"`
	ForkContentDigest     string         `json:"fork_content_digest,omitempty"`
	Operation             *SyncOperation `json:"operation,omitempty"`
}

// SyncView is a read-only catalog summary populated off the TUI event loop.
type SyncView struct {
	Mode, ForkState, ForkPath, UpstreamPath, BaselineVersion, BaselineState, Customization, AttemptPath, OperationID, OperationState string
	NextActions                                                                                                                      []string
	RollbackAvailable, RecoveryRequired, CleanupPending                                                                              bool
}
