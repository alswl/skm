package engines

import "github.com/alswl/skm/skm/pkg/common"

type SyncStatus struct {
	AttemptState   string       `json:"attempt_state"`
	AttemptVersion string       `json:"attempt_version,omitempty"`
	Failures       []SyncChange `json:"failure_paths,omitempty"`
	NextActions    []string     `json:"next_actions,omitempty"`

	Mode              string         `json:"mode"`
	ForkState         string         `json:"fork_state"`
	ActivePath        string         `json:"active_path"`
	UpstreamPath      string         `json:"upstream_path"`
	ForkPath          string         `json:"fork_path,omitempty"`
	BaselineState     string         `json:"baseline_state"`
	BaselineVersion   string         `json:"baseline_version,omitempty"`
	Customization     string         `json:"customization"`
	AttemptPath       string         `json:"attempt_path,omitempty"`
	RollbackAvailable bool           `json:"rollback_available"`
	Origin            *common.Origin `json:"origin,omitempty"`
	Changes           []SyncChange   `json:"changes"`
}
type SyncChange struct {
	MarkerLabel string `json:"marker_label,omitempty"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`
}
type SyncOptions struct {
	DryRun   bool
	Replace  bool
	Upstream bool
	Accept   bool
	Abort    bool
	Rollback bool
}
type SyncAttempt struct {
	Schema         int          `json:"schema_version"`
	Entry          string       `json:"entry"`
	RecordDigest   string       `json:"record_digest"`
	UpstreamDigest string       `json:"upstream_digest"`
	LocalDigest    string       `json:"local_digest"`
	Version        string       `json:"version"`
	BaseDigest     string       `json:"base_digest"`
	NewDigest      string       `json:"new_digest"`
	Failures       []SyncChange `json:"failures"`
}
