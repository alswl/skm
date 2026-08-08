package managers

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/alswl/skm/skm/pkg/common"
)

// ExportResult carries the emitted deploy command (contract/cli-json.md). The
// command is empty when nothing is installed (SC-008).
type ExportResult struct {
	Command string `json:"command"`
}

// Export emits a quote-safe `skm deploy ...` command scoped to the actually
// installed active assets and their targets (FR-028). It emits no command when
// the installed set is empty, to avoid an unscoped command installing
// everything.
func (s *Services) Export() (*ExportResult, error) {
	type ref struct{ name, target string }
	var refs []ref
	for _, e := range s.Scan() {
		if e.Status != common.StatusActive {
			continue
		}
		for _, t := range s.Installer.Targets(e) {
			if s.Installer.State(e, t) == common.InstallInstalled {
				refs = append(refs, ref{name: e.Name, target: t.Name})
			}
		}
	}
	if len(refs) == 0 {
		return &ExportResult{}, nil
	}

	repo := s.repoOrigin()
	if repo == "" {
		return nil, common.WithExitCode(
			fmt.Errorf("export: the repository has no git origin; use --repo to provide one"), common.ExitError)
	}

	targetSet := map[string]bool{}
	nameSet := map[string]bool{}
	for _, r := range refs {
		targetSet[r.target] = true
		nameSet[r.name] = true
	}
	targets := sortedKeys(targetSet)
	names := sortedKeys(nameSet)

	cmd := "skm deploy" +
		" --repo " + shellQuote(repo) +
		" --target " + shellQuote(strings.Join(targets, ",")) +
		" --only " + shellQuote(strings.Join(names, ","))
	return &ExportResult{Command: cmd}, nil
}

// repoOrigin returns the git remote origin URL of the repository, or "".
func (s *Services) repoOrigin() string {
	out, err := exec.Command("git", "-C", s.Cfg.Root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
