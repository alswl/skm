package installer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// TargetPlugin adapts an executable implementing the Target plugin subprocess
// protocol, symmetric to providers.PluginProvider: a Target that can't be
// expressed with the three built-in strategies (skill-symlink,
// command-marker, command-adapter) is installed into via an out-of-process
// executable instead, referenced from targets.json as strategy
// "plugin:<id>" (common.InstallStrategy.IsPlugin/PluginID).
type TargetPlugin struct {
	path  string
	id    string
	label string
	mu    sync.Mutex
}

// targetPluginTimeout bounds every subprocess call so one slow/hung plugin
// cannot stall install/uninstall/state or discovery. A var (not const) so
// tests can shrink it.
var targetPluginTimeout = 15 * time.Second

// TargetPluginError is a diagnosable error from a Target plugin operation: a
// stable code plus a human message.
type TargetPluginError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *TargetPluginError) Error() string { return e.Message }

const (
	CodeTargetProtocolError   = "protocol_error"
	CodeTargetTimeout         = "timeout"
	CodeTargetDuplicateID     = "duplicate_id"
	CodeTargetEmptyID         = "empty_id"
	CodeTargetInstallFailed   = "install_failed"
	CodeTargetUninstallFailed = "uninstall_failed"
	CodeTargetStateFailed     = "state_failed"
	CodeTargetNotLoaded       = "not_loaded"
)

// TargetPluginCapability describes what a Target plugin handles, shown by
// `target plugin list` without installing anything. A plugin that doesn't
// implement the `capability` action falls back to {ID,Label,"",nil}.
type TargetPluginCapability struct {
	ID          string
	Label       string
	Description string
	Kinds       []common.EntryKind
}

// PluginLoadFailure records why a Target plugin failed to load during
// discovery, mirroring providers.ProviderLoadFailure.
type PluginLoadFailure struct {
	Path   string
	ID     string
	Reason TargetPluginError
}

type targetPluginRequest struct {
	Action     string           `json:"action"`
	Name       string           `json:"name,omitempty"`
	Kind       common.EntryKind `json:"kind,omitempty"`
	SourcePath string           `json:"source_path,omitempty"`
	TargetPath string           `json:"target_path,omitempty"`
	Force      bool             `json:"force,omitempty"`
}

// targetPluginErrorField accepts either the new {code,message} object or a
// legacy bare string (mapped to CodeTargetInstallFailed), same convention as
// providers.pluginError.
type targetPluginErrorField struct {
	Code    string
	Message string
}

func (e *targetPluginErrorField) UnmarshalJSON(data []byte) error {
	var obj struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(data, &obj); err == nil && (obj.Code != "" || obj.Message != "") {
		e.Code, e.Message = obj.Code, obj.Message
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	e.Code, e.Message = CodeTargetInstallFailed, s
	return nil
}

type targetPluginResponse struct {
	ID          string                  `json:"id,omitempty"`
	Label       string                  `json:"label,omitempty"`
	Description string                  `json:"description,omitempty"`
	Kinds       []common.EntryKind      `json:"kinds,omitempty"`
	Result      *bool                   `json:"result,omitempty"`
	Path        string                  `json:"path,omitempty"`
	State       string                  `json:"state,omitempty"`
	Error       *targetPluginErrorField `json:"error,omitempty"`
}

// NewTargetPlugin loads a plugin executable, probing its id and label.
func NewTargetPlugin(path string) (*TargetPlugin, error) {
	p := &TargetPlugin{path: path}
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	idResp, err := p.call(ctx, targetPluginRequest{Action: "id"})
	if err != nil {
		return nil, fmt.Errorf("target plugin %s: %w", path, err)
	}
	if idResp.ID == "" {
		return nil, &TargetPluginError{Code: CodeTargetEmptyID, Message: fmt.Sprintf("target plugin %s: returned an empty id", path)}
	}
	p.id = idResp.ID
	if lbl, err := p.call(ctx, targetPluginRequest{Action: "label"}); err == nil {
		p.label = lbl.Label
	}
	return p, nil
}

// ID returns the plugin's stable identifier (the "plugin:<id>" strategy
// suffix that refers to it).
func (p *TargetPlugin) ID() string { return p.id }

// Label returns the human label (falling back to the id).
func (p *TargetPlugin) Label() string {
	if p.label == "" {
		return p.id
	}
	return p.label
}

// Capability runs the plugin's optional `capability` action, falling back to
// {id,label,"",nil} when unimplemented.
func (p *TargetPlugin) Capability() TargetPluginCapability {
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, targetPluginRequest{Action: "capability"})
	if err != nil || resp.Error != nil {
		return TargetPluginCapability{ID: p.ID(), Label: p.Label()}
	}
	cap := TargetPluginCapability{ID: p.ID(), Label: p.Label(), Description: resp.Description, Kinds: resp.Kinds}
	if resp.ID != "" {
		cap.ID = resp.ID
	}
	if resp.Label != "" {
		cap.Label = resp.Label
	}
	return cap
}

// Install asks the plugin to place entry into target, reporting whether
// anything changed. The plugin performs its own filesystem I/O — it is not
// wrapped by dal.FileTransaction, the same trust boundary providers.Fetch
// already has.
func (p *TargetPlugin) Install(entry *common.Entry, target common.InstallTarget, force bool) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, targetPluginRequest{
		Action: "install", Name: entry.Name, Kind: entry.Kind,
		SourcePath: entry.Path, TargetPath: target.Path, Force: force,
	})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.id, resp.Error.Message)}
	}
	return resp.Result != nil && *resp.Result, nil
}

// Uninstall asks the plugin to remove its managed install of entry from
// target, reporting whether anything changed.
func (p *TargetPlugin) Uninstall(entry *common.Entry, target common.InstallTarget) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, targetPluginRequest{
		Action: "uninstall", Name: entry.Name, Kind: entry.Kind, TargetPath: target.Path,
	})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.id, resp.Error.Message)}
	}
	return resp.Result != nil && *resp.Result, nil
}

// State asks the plugin to classify entry's install health within target.
func (p *TargetPlugin) State(entry *common.Entry, target common.InstallTarget) (common.InstallState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, targetPluginRequest{
		Action: "state", Name: entry.Name, Kind: entry.Kind,
		SourcePath: entry.Path, TargetPath: target.Path,
	})
	if err != nil {
		return common.InstallAbsent, err
	}
	if resp.Error != nil {
		return common.InstallAbsent, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.id, resp.Error.Message)}
	}
	switch common.InstallState(resp.State) {
	case common.InstallInstalled, common.InstallConflict, common.InstallDangling, common.InstallAbsent:
		return common.InstallState(resp.State), nil
	default:
		return common.InstallAbsent, nil
	}
}

func (p *TargetPlugin) call(ctx context.Context, req targetPluginRequest) (*targetPluginResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var resp targetPluginResponse
	if err := plugins.Call(ctx, "target plugin", p.path, req, &resp); err != nil {
		var ce *plugins.CallError
		if errors.As(err, &ce) {
			code := CodeTargetProtocolError
			if ce.Kind == plugins.KindTimeout {
				code = CodeTargetTimeout
			}
			return nil, &TargetPluginError{Code: code, Message: ce.Message}
		}
		return nil, err
	}
	return &resp, nil
}
