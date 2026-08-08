package common

import (
	"errors"
	"fmt"
)

// Exit codes (FR-032). os.Exit is reserved for main; these are attached to
// errors and interpreted at the top level.
const (
	ExitOK     = 0 // success, or completed with warnings only
	ExitObject = 1 // problem with the checked/operated object
	ExitError  = 2 // argument / tool / provider / execution error
)

// ExitCoder is implemented by errors that carry an explicit exit code.
type ExitCoder interface {
	ExitCode() int
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }
func (e *exitError) ExitCode() int { return e.code }

// WithExitCode wraps err so that the CLI exits with the given code. A nil err
// returns nil.
func WithExitCode(err error, code int) error {
	if err == nil {
		return nil
	}
	return &exitError{code: code, err: err}
}

// ExitCodeOf returns the exit code attached to err via WithExitCode, or
// defaultCode when none is attached.
func ExitCodeOf(err error, defaultCode int) int {
	var c ExitCoder
	if errors.As(err, &c) {
		return c.ExitCode()
	}
	return defaultCode
}

// Errf wraps a format error using %w for chain preservation.
func Errf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// NeedsForcer is implemented by errors that would succeed if the same
// operation were retried with force enabled (a same-named non-managed object
// exists at the destination).
type NeedsForcer interface {
	NeedsForce() bool
}

type needsForceError struct{ err error }

func (e *needsForceError) Error() string    { return e.err.Error() }
func (e *needsForceError) Unwrap() error    { return e.err }
func (e *needsForceError) NeedsForce() bool { return true }

// WithNeedsForce marks err as resolvable by retrying with force=true, so a
// caller (e.g. the TUI) can offer a confirm-then-retry path instead of a dead
// end. A nil err returns nil.
func WithNeedsForce(err error) error {
	if err == nil {
		return nil
	}
	return &needsForceError{err: err}
}

// IsNeedsForce reports whether err (or any error it wraps) was marked via
// WithNeedsForce.
func IsNeedsForce(err error) bool {
	var nf NeedsForcer
	return errors.As(err, &nf) && nf.NeedsForce()
}
