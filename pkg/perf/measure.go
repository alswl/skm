package perf

import (
	"time"

	"github.com/alswl/skm/skm/pkg/common"
	"github.com/alswl/skm/skm/pkg/dal"
	"github.com/alswl/skm/skm/pkg/managers/installer"
	"github.com/alswl/skm/skm/pkg/managers/repository"
)

// Measurement records the elapsed time for a named operation.
type Measurement struct {
	Name    string        `json:"name"`
	Elapsed time.Duration `json:"elapsed_ns"`
}

// Suite measures the key operations against a repository: scan, origin lookup,
// content hash, install-state derivation, and the read report data.
func Suite(root string, targets []common.InstallTarget) []Measurement {
	repo := repository.New(root)
	inst := installer.New(targets, nil)
	entries := repo.Scan() // warm cache for repeatable timing

	var out []Measurement
	out = append(out, Measure("scan", 10, func() {
		_ = repo.Scan()
	}))

	out = append(out, Measure("origin-lookup", 50, func() {
		if len(entries) > 0 {
			_, _ = dal.ReadMeta(entries[0].Path)
		}
	}))

	out = append(out, Measure("content-hash", 20, func() {
		if len(entries) > 0 {
			_, _ = dal.DirHash(entries[0].Path, nil)
		}
	}))

	out = append(out, Measure("install-state", 50, func() {
		for _, e := range entries {
			for _, t := range inst.Targets(e) {
				_ = inst.State(e, t)
			}
		}
	}))

	entry := firstActive(entries)
	if entry != nil {
		out = append(out, Measure("origin-lookup-entry", 100, func() {
			_, _ = dal.ReadMeta(entry.Path)
		}))
	}

	return out
}

// Measure times fn over ops repetitions.
func Measure(name string, ops int, fn func()) Measurement {
	start := time.Now()
	for i := 0; i < ops; i++ {
		fn()
	}
	return Measurement{Name: name, Elapsed: time.Duration(int64(time.Since(start)) / int64(ops))}
}

func firstActive(entries []*common.Entry) *common.Entry {
	for _, e := range entries {
		if e.Status == common.StatusActive {
			return e
		}
	}
	return nil
}
