package version

import "testing"

func TestBuildDateUsesExplicitValue(t *testing.T) {
	previous := Date
	t.Cleanup(func() { Date = previous })
	Date = "2026-08-08T00:00:00Z"
	if got := BuildDate(); got != Date {
		t.Fatalf("BuildDate() = %q, want explicit %q", got, Date)
	}
}
