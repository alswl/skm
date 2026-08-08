package common

import (
	"errors"
	"testing"
)

func TestWithExitCode(t *testing.T) {
	if WithExitCode(nil, ExitError) != nil {
		t.Errorf("WithExitCode(nil, ...) should return nil")
	}
	base := errors.New("boom")
	wrapped := WithExitCode(base, ExitObject)
	if wrapped.Error() != "boom" {
		t.Errorf("wrapped error message = %q, want %q", wrapped.Error(), "boom")
	}
	if !errors.Is(wrapped, base) {
		t.Errorf("wrapped error should unwrap to the original")
	}
}

func TestExitCodeOf(t *testing.T) {
	base := errors.New("boom")
	if got := ExitCodeOf(base, ExitError); got != ExitError {
		t.Errorf("ExitCodeOf(plain error) = %d, want default %d", got, ExitError)
	}
	wrapped := WithExitCode(base, ExitObject)
	if got := ExitCodeOf(wrapped, ExitError); got != ExitObject {
		t.Errorf("ExitCodeOf(wrapped error) = %d, want attached %d", got, ExitObject)
	}
}
