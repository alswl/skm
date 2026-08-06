package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// PluginProvider adapts an executable implementing the subprocess JSON
// protocol (research R8 / FR-035). Each request is a single JSON line on
// stdin; the response is a single JSON line on stdout.
type PluginProvider struct {
	path  string
	id    string
	label string
	mu    sync.Mutex
}

type pluginRequest struct {
	Action  string `json:"action"`
	Address string `json:"address,omitempty"`
}

type pluginResponse struct {
	ID     string `json:"id,omitempty"`
	Label  string `json:"label,omitempty"`
	Result *bool  `json:"result,omitempty"`
	Path   string `json:"path,omitempty"`
	Error  string `json:"error,omitempty"`
}

// NewPluginProvider loads a plugin executable, probing its id and label.
func NewPluginProvider(path string) (*PluginProvider, error) {
	p := &PluginProvider{path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	idResp, err := p.call(ctx, "id", "")
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", path, err)
	}
	if idResp.ID == "" {
		return nil, fmt.Errorf("plugin %s: returned an empty id", path)
	}
	p.id = idResp.ID
	if lbl, err := p.call(ctx, "label", ""); err == nil {
		p.label = lbl.Label
	}
	return p, nil
}

// ID returns the plugin's provider id.
func (p *PluginProvider) ID() string { return p.id }

// Label returns the human label (falling back to the id).
func (p *PluginProvider) Label() string {
	if p.label == "" {
		return p.id
	}
	return p.label
}

// CanHandle runs the plugin's can_handle for the address.
func (p *PluginProvider) CanHandle(address string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	if resp.Error != "" {
		return "", fmt.Errorf("plugin %s: %s", p.id, resp.Error)
	}
	if resp.Path == "" {
		return "", fmt.Errorf("plugin %s: fetch returned no path", p.id)
	}
	return resp.Path, nil
}

func (p *PluginProvider) call(ctx context.Context, action, address string) (*pluginResponse, error) {
	req := pluginRequest{Action: action, Address: address}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cmd := exec.CommandContext(ctx, p.path)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", p.path, err)
	}
	var resp pluginResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("plugin %s: invalid JSON response: %w", p.path, err)
	}
	return &resp, nil
}
