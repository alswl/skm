package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alswl/skm/skm/pkg/plugins"
	"github.com/alswl/skm/skm/pkg/providers"
)

// PluginProvider adapts an executable implementing the subprocess JSON
// protocol (research R8 / FR-035, extended per contracts/provider-protocol.md
// for 002-open-provider-target). Each request is a single JSON line on
// stdin; the response is a single JSON line on stdout.
type PluginProvider struct {
	*plugins.Process
}

// pluginTimeout bounds every subprocess call so one slow/hung plugin cannot
// stall startup, provider list, or an import (FR-006, research R4). A var
// (not const) so tests can shrink it to exercise timeout isolation quickly.
var pluginTimeout = 15 * time.Second

type pluginRequest struct {
	Action  string `json:"action"`
	Address string `json:"address,omitempty"`
}

// pluginError tolerates the legacy bare-string error form already-built
// plugins emit (contracts/provider-protocol.md).
type pluginError struct {
	Code    string
	Message string
}

func (e *pluginError) UnmarshalJSON(data []byte) error {
	return plugins.UnmarshalError(data, &e.Code, &e.Message, providers.CodeFetchFailed)
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
	p := &PluginProvider{plugins.NewProcess("plugin", path)}
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	err := p.Handshake(func(action string) (plugins.Identity, error) {
		resp, err := p.call(ctx, action, "")
		if err != nil {
			return plugins.Identity{}, err
		}
		return plugins.Identity{ID: resp.ID, Label: resp.Label, ProtocolVersion: resp.ProtocolVersion}, nil
	})
	switch {
	case errors.Is(err, plugins.ErrEmptyID):
		return nil, &providers.ProviderError{Code: providers.CodeEmptyID, Message: fmt.Sprintf("plugin %s: returned an empty id", path)}
	case err != nil:
		return nil, fmt.Errorf("plugin %s: %w", path, err)
	}
	return p, nil
}

// Capability runs the plugin's optional `capability` action. A plugin that
// doesn't implement it (error or empty response) falls back to
// {id,label,"",nil} (contracts/provider-protocol.md).
func (p *PluginProvider) Capability() providers.Capability {
	ctx, cancel := context.WithTimeout(context.Background(), pluginTimeout)
	defer cancel()
	resp, err := p.call(ctx, "capability", "")
	if err != nil || resp.Error != nil {
		return providers.Capability{ID: p.ID(), Label: p.Label()}
	}
	cap := providers.Capability{ID: p.ID(), Label: p.Label(), Description: resp.Description, Schemes: resp.Schemes, Icon: resp.Icon}
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
		return "", &providers.ProviderError{Code: resp.Error.Code, Message: fmt.Sprintf("plugin %s: %s", p.ID(), resp.Error.Message)}
	}
	if resp.Path == "" {
		return "", &providers.ProviderError{Code: providers.CodeFetchFailed, Message: fmt.Sprintf("plugin %s: fetch returned no path", p.ID())}
	}
	return resp.Path, nil
}

func (p *PluginProvider) call(ctx context.Context, action, address string) (*pluginResponse, error) {
	var resp pluginResponse
	if err := p.Call(ctx, pluginRequest{Action: action, Address: address}, &resp); err != nil {
		var ce *plugins.CallError
		if errors.As(err, &ce) {
			return nil, &providers.ProviderError{Code: ce.Code(), Message: ce.Message}
		}
		return nil, err
	}
	return &resp, nil
}
