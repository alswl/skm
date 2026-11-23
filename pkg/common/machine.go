package common

import "errors"

// ErrMachineRenameUnavailable indicates unlink cannot be safely previewed.
var ErrMachineRenameUnavailable = errors.New("exclusive rename capability unavailable")

// Directory identifies a registered directory. Tags are names, not
// separate persisted entities or identifiers.
type Directory struct {
	Tags []string `yaml:"tags" json:"tags"`
}

// InstallationScope identifies where a skill should be installed.
type InstallationScope struct {
	Kind string `yaml:"kind" json:"kind"`
	Name string `yaml:"name,omitempty" json:"name,omitempty"`
}

// Installation describes a desired skill installation.
type Installation struct {
	Skill  string            `yaml:"skill" json:"skill"`
	Target string            `yaml:"target" json:"target"`
	Scope  InstallationScope `yaml:"scope" json:"scope"`
}

// InstallationObservation describes a current on-disk location.
type InstallationObservation struct {
	Scope          string   `json:"scope"`
	Directory      string   `json:"directory"`
	Target         string   `json:"target"`
	Path           string   `json:"path"`
	Skill          string   `json:"skill,omitempty"`
	ExpectedSkills []string `json:"expected_skills,omitempty"`
	Status         string   `json:"status"`
	Error          string   `json:"error,omitempty"`
	Declared       bool     `json:"declared"`
}

// InstallationPlanItem describes an installation at one physical location.
type InstallationPlanItem struct {
	Directory string `json:"directory"`
	Target    string `json:"target"`
	Skill     string `json:"skill"`
	Path      string `json:"path,omitempty"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
}
