package services

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alswl/skm/skm/pkg/dal"
)

// NewSkillsSh returns the built-in skills.sh provider (git-backed,
// "skills.sh://<owner>/<repo>"; host overridable via SKM_SKILLS_SH_HOST).
// skills.sh indexes skills living in public GitHub repos, so the default host
// is github.com; the scheme keeps the skills.sh identity while the actual
// clone happens against the (overridable) host.
func NewSkillsSh() Provider {
	return gitBackedProvider{
		id: "skills-sh", label: "Skills.sh", scheme: "skills.sh://",
		envHostVar: "SKM_SKILLS_SH_HOST", defaultHost: "github.com",
		icon: "🌐",
	}
}

// skillsShShortcut names a skill by its short directory name rather than its
// exact path inside a repo — the shape both forms skills.sh shows on its own
// site use, neither of which is skm's own "skills.sh://" scheme.
type skillsShShortcut struct {
	repoURL, name string
}

// npxSkillsAddRe matches the install command every skills.sh skill page
// shows verbatim, e.g. "npx skills add https://github.com/owner/repo --skill
// name" (an optional leading "$ " is the shell prompt users often copy
// along with it).
var npxSkillsAddRe = regexp.MustCompile(`^\$?\s*npx\s+skills\s+add\s+(\S+)\s+--skill\s+(\S+)\s*$`)

// skillsShPageURLRe matches a skills.sh skill page URL itself, e.g.
// "https://skills.sh/owner/repo/name" — the same (owner, repo, name) triple
// the npx command carries.
var skillsShPageURLRe = regexp.MustCompile(`^https://skills\.sh/([^/\s]+)/([^/\s]+)/([^/\s]+)/?$`)

// parseSkillsShShortcut recognizes the npx command line or page URL forms;
// both name a skill skills.sh's own tooling resolves by searching the repo
// for a directory with that name, since neither carries the skill's actual
// path inside the repo (that can be nested arbitrarily deep — see
// mattpocock/skills/skills/productivity/grill-me vs. the "grill-me" name
// both forms give).
func parseSkillsShShortcut(address string) *skillsShShortcut {
	address = strings.TrimSpace(address)
	if m := npxSkillsAddRe.FindStringSubmatch(address); m != nil {
		return &skillsShShortcut{repoURL: strings.TrimSuffix(m[1], ".git") + ".git", name: m[2]}
	}
	if m := skillsShPageURLRe.FindStringSubmatch(address); m != nil {
		return &skillsShShortcut{repoURL: fmt.Sprintf("https://github.com/%s/%s.git", m[1], m[2]), name: m[3]}
	}
	return nil
}

// findSkillDirectory walks a cloned repo for a directory named exactly name
// that also contains a skill or command marker — the same lookup skills.sh's
// own npx tool performs, needed because neither shortcut form above carries
// the directory's actual path inside the repo.
func findSkillDirectory(root, name string) (string, error) {
	var matches []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return fs.SkipDir
		}
		if d.Name() == name &&
			(dal.PathExists(filepath.Join(path, "SKILL.md")) || dal.PathExists(filepath.Join(path, "COMMAND.md"))) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search for %q: %w", name, err)
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no skill or command directory named %q found in this repository", name)
	case 1:
		return matches[0], nil
	default:
		rel := make([]string, len(matches))
		for i, m := range matches {
			r, _ := filepath.Rel(root, m)
			rel[i] = r
		}
		return "", fmt.Errorf("%q matches multiple directories (%s); use the exact skills.sh://owner/repo/path form instead",
			name, strings.Join(rel, ", "))
	}
}
