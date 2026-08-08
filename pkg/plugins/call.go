package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// CallErrorKind classifies a subprocess call failure so the caller can map it
// into its own domain error type/code.
type CallErrorKind string

const (
	KindTimeout  CallErrorKind = "timeout"
	KindProtocol CallErrorKind = "protocol_error"
)

// CallError is a classified failure from Call.
type CallError struct {
	Kind    CallErrorKind
	Message string
}

func (e *CallError) Error() string { return e.Message }

// Call executes path as a subprocess implementing the JSON-over-stdin/stdout
// plugin protocol: req is marshaled to JSON and written to stdin, and the
// process's stdout is unmarshaled as JSON into resp. label prefixes error
// messages (e.g. "plugin" or "target plugin") to match each domain's
// existing wording. The caller is responsible for bounding ctx with a
// timeout where one is wanted — Call does not impose one itself.
func Call(ctx context.Context, label, path string, req, resp any) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path)
	cmd.Stdin = bytes.NewReader(data)
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return &CallError{Kind: KindTimeout, Message: fmt.Sprintf("%s %s: timed out", label, path)}
		}
		return &CallError{Kind: KindProtocol, Message: fmt.Sprintf("%s %s: %s", label, path, err)}
	}
	if err := json.Unmarshal(out, resp); err != nil {
		return &CallError{Kind: KindProtocol, Message: fmt.Sprintf("%s %s: invalid JSON response: %s", label, path, err)}
	}
	return nil
}
