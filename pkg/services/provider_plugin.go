package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alswl/skm/skm/pkg/plugins"
)

// PluginProvider adapts an executable implementing the subprocess JSON
// protocol (research R8 / FR-035, extended per contracts/provider-protocol.md
// for 002-open-provider-target). Each request is a single JSON line on
// stdin; the response is a single JSON line on stdout.
type PluginProvider struct {
	path            string
	id              string
	label           string
	protocolVersion int // declared in the id response; 1 (baseline) when absent
	mu              sync.Mutex
}

// pluginTimeout bounds every subprocess call so one slow/hung plugin cannot
// stall startup, provider list, or an import (FR-006, research R4). A var
// (not const) so tests can shrink it to exercise timeout isolation quickly.
var pluginTimeout = 15 * time.Second

type pluginRequest struct {
	Action  string `json:"action"`
	Address string `json:"address,omitempty"`
}

// pluginError accepts either the new {code,message} object or a legacy bare
// string (mapped to CodeFetchFailed) for backward compatibility with
// already-built plugins (contracts/provider-protocol.md).
type pluginError struct {
	Code    string
	Message string
}

func (e *pluginError) UnmarshalJSON(data []byte) error {
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
	e.Code, e.Message = CodeFetchFailed, s
	return nil
}

type pluginResponse struct {
	ID              string       `json:"id,omitempty"`
	ProtocolVersion int          `json:"protocol_version,omitempty"`
	Label           string       `json:"label,omitempty"`
	Description     string       `json:"description,omitempty"`
	Schemes         []string     `json:"schemes,omitempty"`
	Icon            string       `json:"icon,omitempty"`
	Address         string       `json:"address,omitempty"`
	Result          *bool        `json:"result,omitempty"`
	Path            string       `json:"path,omitempty"`
	Error           *pluginError `json:"error,omitempty"`
}

// NewPluginProvider loads a plugin executable, probing its id and label.
func NewPluginProvider(path string) (*PluginProvider, error) {
	p := &PluginProvider{path: path}
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	idResp, err := p.call(ctx, "id", "")
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", path, err)
	}
	if idResp.ID == "" {
		return nil, &ProviderError{Code: CodeEmptyID, Message: fmt.Sprintf("plugin %s: returned an empty id", path)}
	}
	p.id = idResp.ID
	// An undeclared protocol_version is the v1 baseline (plugin_host.go).
	p.protocolVersion = idResp.ProtocolVersion
	if p.protocolVersion == 0 {
		p.protocolVersion = 1
	}
	if lbl, err := p.call(ctx, "label", ""); err == nil {
		p.label = lbl.Label
	}
	return p, nil
}

// ProtocolVersion returns the protocol version the plugin declares it
// implements (1 when it predates the versioning field).
func (p *PluginProvider) ProtocolVersion() int { return p.protocolVersion }

// ID returns the plugin's provider id.
func (p *PluginProvider) ID() string { return p.id }

// Descriptor exposes the same protocol identity as in-process providers.
func (p *PluginProvider) Descriptor() PluginDescriptor {
	cap := p.Capability()
	return PluginDescriptor{Version: p.protocolVersion, Kind: PluginKindProvider, ID: p.ID(), Label: p.Label(), Description: cap.Description, Path: p.path}
}

// Handle is the common plugin-host entry point. The subprocess wire format is
// still the established v1 provider protocol; call translates it into the v2
// in-process envelope so callers never branch on plugin origin.
func (p *PluginProvider) Handle(ctx context.Context, req PluginRequest) (PluginResponse, error) {
	action := req.Action
	if action == "describe" {
		return PluginResponse{Descriptor: p.Descriptor()}, nil
	}
	resp, err := p.call(ctx, action, req.Address)
	if err != nil {
		return PluginResponse{}, err
	}
	if resp.Error != nil {
		return PluginResponse{}, &ProviderError{Code: resp.Error.Code, Message: fmt.Sprintf("plugin %s: %s", p.id, resp.Error.Message)}
	}
	out := PluginResponse{Address: resp.Address, Path: resp.Path, Result: resp.Result}
	if action == "capability" {
		out.Capability = Capability{ID: p.ID(), Label: p.Label(), Description: resp.Description, Schemes: resp.Schemes, Icon: resp.Icon}
		if resp.ID != "" {
			out.Capability.ID = resp.ID
		}
		if resp.Label != "" {
			out.Capability.Label = resp.Label
		}
	}
	return out, nil
}

// Label returns the human label (falling back to the id).
func (p *PluginProvider) Label() string {
	if p.label == "" {
		return p.id
	}
	return p.label
}

// Capability runs the plugin's optional `capability` action. A plugin that
// doesn't implement it (error or empty response) falls back to
// {id,label,"",nil} (contracts/provider-protocol.md).
func (p *PluginProvider) Capability() Capability {
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, "capability", "")
	if err != nil || resp.Error != nil {
		return Capability{ID: p.ID(), Label: p.Label()}
	}
	cap := Capability{ID: p.ID(), Label: p.Label(), Description: resp.Description, Schemes: resp.Schemes, Icon: resp.Icon}
	if resp.ID != "" {
		cap.ID = resp.ID
	}
	if resp.Label != "" {
		cap.Label = resp.Label
	}
	return cap
}

// Normalize runs the plugin's optional `normalize` action. A plugin that
// doesn't implement it, errors, or returns an empty address falls back to
// identity (address unchanged).
func (p *PluginProvider) Normalize(address string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, "normalize", address)
	if err != nil || resp.Error != nil || resp.Address == "" {
		return address, nil
	}
	return resp.Address, nil
}

// CanHandle runs the plugin's can_handle for the address.
func (p *PluginProvider) CanHandle(address string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, "can_handle", address)
	return err == nil && resp.Result != nil && *resp.Result
}

// Fetch runs the plugin's fetch and returns the staged path it produced. The
// caller owns cleanup of the returned directory.
func (p *PluginProvider) Fetch(ctx context.Context, address string) (string, error) {
	resp, err := p.call(ctx, "fetch", address)
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", &ProviderError{Code: resp.Error.Code, Message: fmt.Sprintf("plugin %s: %s", p.id, resp.Error.Message)}
	}
	if resp.Path == "" {
		return "", &ProviderError{Code: CodeFetchFailed, Message: fmt.Sprintf("plugin %s: fetch returned no path", p.id)}
	}
	return resp.Path, nil
}

func (p *PluginProvider) call(ctx context.Context, action, address string) (*pluginResponse, error) {
	req := pluginRequest{Action: action, Address: address}
	p.mu.Lock()
	defer p.mu.Unlock()
	var resp pluginResponse
	if err := plugins.Call(ctx, "plugin", p.path, req, &resp); err != nil {
		var ce *plugins.CallError
		if errors.As(err, &ce) {
			code := CodeProtocolError
			if ce.Kind == plugins.KindTimeout {
				code = CodeTimeout
			}
			return nil, &ProviderError{Code: code, Message: ce.Message}
		}
		return nil, err
	}
	return &resp, nil
}
