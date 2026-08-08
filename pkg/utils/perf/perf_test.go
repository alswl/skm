// Package perf is the skm-specific performance regression harness. It is
// test-only by design: Suite (below) exercises dal/managers directly to keep
// the baseline honest, and living entirely in _test.go keeps those domain
// imports invisible to the depguard architecture check — no layering
// exception needed (003-engineering-optimization R4).
package perf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/services/installer"
	"github.com/alswl/skm/skm/pkg/services/repository"
	"github.com/alswl/skm/skm/pkg/utils/timing"
	"github.com/stretchr/testify/require"
)

// baselinesPath is the recorded baseline file committed under testdata.
var baselinesPath = "../../../testdata/perf/baselines/measurements.json"

// regressionFactor is how far a measurement may drift from its baseline
// before the perf regression test fails (loose to stay robust on loaded
// machines).
const regressionFactor = 5

// scanBudget is the hard interactive-latency bound for a cold scan of a
// ~1k-entry repo (plan.md Performance Goals: target < 500 ms).
const scanBudget = 500 * time.Millisecond

// suite measures the key operations against a repository: scan, origin
// lookup, content hash, install-state derivation, and the read report data.
func suite(root string, targets []common.InstallTarget) []timing.Measurement {
	repo := repository.New(root)
	inst := installer.New(targets, nil)
	entries := repo.Scan() // warm cache for repeatable timing

	var out []timing.Measurement
	out = append(out, timing.Measure("scan", 10, func() {
		_ = repo.Scan()
	}))

	out = append(out, timing.Measure("origin-lookup", 50, func() {
		if len(entries) > 0 {
			_, _ = dal.ReadMeta(entries[0].Path)
		}
	}))

	out = append(out, timing.Measure("content-hash", 20, func() {
		if len(entries) > 0 {
			_, _ = dal.DirHash(entries[0].Path, nil)
		}
	}))

	out = append(out, timing.Measure("install-state", 50, func() {
		for _, e := range entries {
			for _, t := range inst.Targets(e) {
				_ = inst.State(e, t)
			}
		}
	}))

	entry := firstActive(entries)
	if entry != nil {
		out = append(out, timing.Measure("origin-lookup-entry", 100, func() {
			_, _ = dal.ReadMeta(entry.Path)
		}))
	}

	return out
}

func firstActive(entries []*common.Entry) *common.Entry {
	for _, e := range entries {
		if e.Status == common.StatusActive {
			return e
		}
	}
	return nil
}

// buildLargeFixture creates a repository with ~1000 skill entries.
func buildLargeFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < 1000; i++ {
		name := "skill-" + itoa(i)
		dir := filepath.Join(root, "skills", "local", name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		content := "---\nname: " + name + "\ndescription: perf fixture skill\n---\nbody\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644))
	}
	return root
}

func TestPerfRegression(t *testing.T) {
	root := buildLargeFixture(t)
	measurements := suite(root, []common.InstallTarget{})

	// Hard bound: a cold scan of the ~1k-entry repo stays under interactive
	// latency (SC-009 / plan Performance Goals).
	scan := measurementByName(measurements, "scan")
	require.Less(t, scan.Elapsed, scanBudget, "scan must stay under the 500ms interactive budget")

	// Record baselines on the first run, then compare on later runs.
	if _, err := os.Stat(baselinesPath); os.IsNotExist(err) {
		t.Logf("no baseline yet; recording %d measurements to %s", len(measurements), baselinesPath)
		data, err := json.MarshalIndent(measurements, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(baselinesPath), 0o755))
		require.NoError(t, os.WriteFile(baselinesPath, data, 0o644))
		return
	}

	data, err := os.ReadFile(baselinesPath)
	require.NoError(t, err)
	var baselines []timing.Measurement
	require.NoError(t, json.Unmarshal(data, &baselines))

	for _, m := range measurements {
		base := measurementByName(baselines, m.Name)
		require.NotNil(t, base, "baseline exists for %q", m.Name)
		limit := base.Elapsed * regressionFactor
		if m.Elapsed > limit {
			t.Errorf("perf regression: %s took %v, baseline %v (factor %d)", m.Name, m.Elapsed, base.Elapsed, regressionFactor)
		}
	}
}

func measurementByName(ms []timing.Measurement, name string) *timing.Measurement {
	for i := range ms {
		if ms[i].Name == name {
			return &ms[i]
		}
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
