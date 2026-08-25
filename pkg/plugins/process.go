package plugins

import (
	"context"
	"errors"
	"sync"
)

// ErrEmptyID is returned by Handshake when the plugin's `id` response carries
// no id. The caller maps it to its own domain error code.
var ErrEmptyID = errors.New("plugin returned an empty id")

// Identity is the part of an `id`/`label` response every plugin kind
// declares, whatever the rest of its protocol looks like.
type Identity struct {
	ID              string
	Label           string
	ProtocolVersion int
}

// Process is one subprocess plugin executable: its probed identity plus
// serialized JSON calls to it. Provider and Target plugins embed it so the
// transport, the identity handshake and the protocol-version baseline exist
// once; only their request/response payloads and error codes differ.
type Process struct {
	kind    string // "plugin" / "target plugin", prefixed onto error messages
	path    string
	id      string
	label   string
	version int
	mu      sync.Mutex
}

// NewProcess returns an unidentified process over path. Call Handshake before
// using it — ID and ProtocolVersion are empty until then.
func NewProcess(kind, path string) *Process {
	return &Process{kind: kind, path: path}
}

// Handshake probes the plugin's id and label through probe, which the caller
// supplies because the request payload is protocol-specific. An undeclared
// protocol_version is the v1 baseline, so plugins built before versioning
// keep loading. A failing label probe is not an error: the id stands in.
func (p *Process) Handshake(probe func(action string) (Identity, error)) error {
	id, err := probe("id")
	if err != nil {
		return err
	}
	if id.ID == "" {
		return ErrEmptyID
	}
	p.id = id.ID
	p.version = id.ProtocolVersion
	if p.version == 0 {
		p.version = 1
	}
	if lbl, err := probe("label"); err == nil {
		p.label = lbl.Label
	}
	return nil
}

func (p *Process) Path() string { return p.path }

func (p *Process) ID() string { return p.id }

// Label is the human label, falling back to the id.
func (p *Process) Label() string {
	if p.label == "" {
		return p.id
	}
	return p.label
}

// ProtocolVersion is 1 when the plugin predates the versioning field.
func (p *Process) ProtocolVersion() int { return p.version }

// Call runs one request/response exchange, serializing concurrent callers so
// a single-threaded plugin never sees interleaved requests.
func (p *Process) Call(ctx context.Context, req, resp any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Call(ctx, p.kind, p.path, req, resp)
}
