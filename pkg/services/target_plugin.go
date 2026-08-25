package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/installer"
	"github.com/alswl/skm/skm/pkg/plugins"
)

// TargetPlugin adapts an executable implementing the Target plugin subprocess
// protocol, symmetric to PluginProvider: a Target that can't be
// expressed with the three built-in strategies (skill-symlink,
// command-marker, command-adapter) is installed into via an out-of-process
// executable instead, referenced from targets.json as strategy
// "plugin:<id>" (common.InstallStrategy.IsPlugin/PluginID).
type TargetPlugin struct {
	*plugins.Process
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
	CodeTargetProtocolError   = plugins.CodeProtocolError
	CodeTargetTimeout         = plugins.CodeTimeout
	CodeTargetDuplicateID     = plugins.CodeDuplicateID
	CodeTargetEmptyID         = plugins.CodeEmptyID
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
// discovery, mirroring ProviderLoadFailure.
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

// targetPluginErrorField tolerates the legacy bare-string error form, same
// convention as pluginError.
type targetPluginErrorField struct {
	Code    string
	Message string
}

func (e *targetPluginErrorField) UnmarshalJSON(data []byte) error {
	return plugins.UnmarshalError(data, &e.Code, &e.Message, CodeTargetInstallFailed)
}

type targetPluginResponse struct {
	ID              string                      `json:"id,omitempty"`
	ProtocolVersion int                         `json:"protocol_version,omitempty"`
	Label           string                      `json:"label,omitempty"`
	Description     string                      `json:"description,omitempty"`
	Kinds           []common.EntryKind          `json:"kinds,omitempty"`
	Result          *bool                       `json:"result,omitempty"`
	Path            string                      `json:"path,omitempty"`
	State           string                      `json:"state,omitempty"`
	Diff            string                      `json:"diff,omitempty"`
	Dangling        []installer.DanglingInstall `json:"dangling,omitempty"`
	Error           *targetPluginErrorField     `json:"error,omitempty"`
}

// NewTargetPlugin loads a plugin executable, probing its id and label.
func NewTargetPlugin(path string) (*TargetPlugin, error) {
	p := &TargetPlugin{plugins.NewProcess("target plugin", path)}
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	err := p.Handshake(func(action string) (plugins.Identity, error) {
		resp, err := p.call(ctx, targetPluginRequest{Action: action})
		if err != nil {
			return plugins.Identity{}, err
		}
		return plugins.Identity{ID: resp.ID, Label: resp.Label, ProtocolVersion: resp.ProtocolVersion}, nil
	})
	switch {
	case errors.Is(err, plugins.ErrEmptyID):
		return nil, &TargetPluginError{Code: CodeTargetEmptyID, Message: fmt.Sprintf("target plugin %s: returned an empty id", path)}
	case err != nil:
		return nil, fmt.Errorf("target plugin %s: %w", path, err)
	}
	return p, nil
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
// wrapped by dal.FileTransaction, the same trust boundary Fetch
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
		return false, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
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
		return false, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
	}
	return resp.Result != nil && *resp.Result, nil
}

// RemoveForeign asks the plugin to remove the non-managed object occupying
// entry's slot (conflict cleanup). Unlike Uninstall it may remove a real
// foreign object, so the caller confirms first. Requires protocol v2; an older
// plugin gets a clear error instead of an opaque protocol failure.
func (p *TargetPlugin) RemoveForeign(entry *common.Entry, target common.InstallTarget) (bool, error) {
	if p.ProtocolVersion() < removeForeignProtocolVersion {
		return false, &TargetPluginError{Code: CodeTargetProtocolError, Message: fmt.Sprintf(
			"target plugin %s is on protocol v%d; remove_foreign (conflict cleanup) needs v%d — update the plugin",
			p.ID(), p.ProtocolVersion(), removeForeignProtocolVersion)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), targetPluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, targetPluginRequest{
		Action: "remove_foreign", Name: entry.Name, Kind: entry.Kind,
		SourcePath: entry.Path, TargetPath: target.Path,
	})
	if err != nil {
		return false, err
	}
	if resp.Error != nil {
		return false, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
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
		return common.InstallAbsent, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
	}
	switch common.InstallState(resp.State) {
	case common.InstallInstalled, common.InstallConflict, common.InstallDangling, common.InstallAbsent:
		return common.InstallState(resp.State), nil
	default:
		return common.InstallAbsent, nil
	}
}

// Diff asks an optional v2 action for a unified preview of replacing the
// target-side object. Older plugins can omit it; callers receive their
// protocol error and can still offer the overwrite confirmation.
func (p *TargetPlugin) Diff(ctx context.Context, entry *common.Entry, target common.InstallTarget) (string, error) {
	resp, err := p.call(ctx, targetPluginRequest{Action: "diff", Name: entry.Name, Kind: entry.Kind, SourcePath: entry.Path, TargetPath: target.Path})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
	}
	return resp.Diff, nil
}

// Inspect asks the optional v2 action to enumerate dangling objects that no
// Entry can address. Older plugins can omit it; the built-in drivers always
// provide this capability for their link-based strategies.
func (p *TargetPlugin) Inspect(ctx context.Context, target common.InstallTarget) ([]installer.DanglingInstall, error) {
	resp, err := p.call(ctx, targetPluginRequest{Action: "inspect", TargetPath: target.Path})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
	}
	for i := range resp.Dangling {
		resp.Dangling[i].TargetName = target.Name
		resp.Dangling[i].Strategy = common.PluginStrategy(p.ID())
	}
	return resp.Dangling, nil
}

// RepairDangling delegates cleanup of an orphan reported by Inspect to the
// plugin that owns the target filesystem layout.
func (p *TargetPlugin) RepairDangling(ctx context.Context, item installer.DanglingInstall, target common.InstallTarget) error {
	resp, err := p.call(ctx, targetPluginRequest{Action: "repair", Name: item.Name, TargetPath: target.Path, Force: true})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return &TargetPluginError{Code: resp.Error.Code, Message: fmt.Sprintf("target plugin %s: %s", p.ID(), resp.Error.Message)}
	}
	return nil
}

func (p *TargetPlugin) call(ctx context.Context, req targetPluginRequest) (*targetPluginResponse, error) {
	var resp targetPluginResponse
	if err := p.Call(ctx, req, &resp); err != nil {
		var ce *plugins.CallError
		if errors.As(err, &ce) {
			return nil, &TargetPluginError{Code: ce.Code(), Message: ce.Message}
		}
		return nil, err
	}
	return &resp, nil
}
