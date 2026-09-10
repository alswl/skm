package providers

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alswl/skm/skm/pkg/dal"
)

var skillsShBuiltinDefinition = BuiltinDefinition{ID: "skills-sh", New: NewSkillsSh}

// NewSkillsSh returns the built-in skills.sh provider (git-backed,
// "skills.sh://<owner>/<repo>"; host overridable via SKM_SKILLS_SH_HOST).
// skills.sh indexes skills living in public GitHub repos, so the default host
// is github.com; the scheme keeps the skills.sh identity while the actual
// clone happens against the (overridable) host.
func NewSkillsSh() Provider {
	return gitBackedProvider{
		id: "skills-sh", label: "Skills.sh", scheme: "skills.sh://",
		envHostVar: "SKM_SKILLS_SH_HOST", defaultHost: "github.com",
		icon: "△",
	}
}

// skillsShShortcut names a skill by its short directory name rather than its
// exact path inside a repo — the shape the copy-paste forms skills.sh shows
// use, neither of which is skm's own "skills.sh://" scheme. An empty name is
// the bare "npx skills add owner/repo" form, which carries no name; Fetch
// resolves it to the repository's sole skill directory.
type skillsShShortcut struct {
	repoURL, name string
}

// parseSkillsShShortcut recognizes the npx command line or page URL forms;
// both name a skill skills.sh's own tooling resolves by searching the repo
// for a directory with that name, since neither carries the skill's actual
// path inside the repo (that can be nested arbitrarily deep — see
// mattpocock/skills/skills/productivity/grill-me vs. the "grill-me" name
// both forms give).
//
// The command form takes owner/repo shorthand or a full git URL, an optional
// --skill <name> (-s), and ignores the npx tool's other flags — skm imports
// one skill at a time. A nil shortcut with a nil error means the
// address is not skills.sh's to handle at all; a non-nil error means it is,
// but is malformed — Fetch surfaces that to the user.
func parseSkillsShShortcut(address string) (*skillsShShortcut, error) {
	address = strings.TrimSpace(address)
	if m := skillsShPageURLRe.FindStringSubmatch(address); m != nil {
		return &skillsShShortcut{
			repoURL: fmt.Sprintf("https://github.com/%s/%s.git", m[1], m[2]),
			name:    m[3],
		}, nil
	}
	fields := strings.Fields(address)
	if len(fields) > 0 && fields[0] == "$" {
		fields = fields[1:]
	}
	if len(fields) < 4 || fields[0] != "npx" || fields[1] != "skills" || fields[2] != "add" {
		return nil, nil
	}
	var names []string
	for i := 3; i < len(fields); i++ {
		if fields[i] != "--skill" && fields[i] != "-s" {
			continue
		}
		for i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
			i++
			names = append(names, fields[i])
		}
	}
	if len(names) > 1 {
		return nil, fmt.Errorf("npx skills add names several skills (%s); skm imports one at a time — pass a single --skill",
			strings.Join(names, ", "))
	}
	repoURL, err := skillsShRepoURL(fields[3])
	if err != nil {
		return nil, err
	}
	name := ""
	if len(names) == 1 {
		name = names[0]
	}
	return &skillsShShortcut{repoURL: repoURL, name: name}, nil
}

// skillsShPageURLRe matches a skills.sh skill page URL itself, e.g.
// "https://skills.sh/owner/repo/name" or "https://www.skills.sh/owner/repo/name"
// (the site itself links to the "www." host) — the same (owner, repo, name)
// triple the npx command carries.
var skillsShPageURLRe = regexp.MustCompile(`^https://(?:www\.)?skills\.sh/([^/\s]+)/([^/\s]+)/([^/\s]+)/?$`)

// skillsShRepoURL resolves an npx source to a clone URL. Owner/repo shorthand
// resolves against github.com — the same host the page-URL form hardcodes,
// since skills.sh indexes skills living in public GitHub repos
// (SKM_SKILLS_SH_HOST only overrides the skills.sh:// scheme form).
func skillsShRepoURL(source string) (string, error) {
	parts := strings.Split(source, "/")
	if isOwnerRepoShorthand(source) && !strings.HasPrefix(parts[0], ".") && !strings.HasPrefix(parts[1], ".") {
		// A relative path like ./my-local-skills happens to satisfy the
		// two-segment shape too; it is not a GitHub shorthand.
		return "https://github.com/" + source + ".git", nil
	}
	// A URL that already ends in .git must not gain a second one.
	if isGitURL(source) {
		return strings.TrimSuffix(source, ".git") + ".git", nil
	}
	return "", fmt.Errorf("npx skills add source %q is neither owner/repo shorthand nor a git URL", source)
}

// findSkillDirectory walks a cloned repo for a directory named exactly name
// that also contains a skill or command marker — the same lookup skills.sh's
// own npx tool performs, needed because neither shortcut form above carries
// the directory's actual path inside the repo.
func findSkillDirectory(root, name string) (string, error) {
	matches, err := skillMarkerDirectories(root)
	if err != nil {
		return "", err
	}
	var named []string
	for _, m := range matches {
		if filepath.Base(m) == name {
			named = append(named, m)
		}
	}
	switch len(named) {
	case 0:
		return "", fmt.Errorf("no skill or command directory named %q found in this repository", name)
	case 1:
		return named[0], nil
	default:
		return "", fmt.Errorf("%q matches multiple directories (%s); use the exact skills.sh://owner/repo/path form instead",
			name, relDirectoryList(root, named))
	}
}

// findSoleSkillDirectory resolves the bare "npx skills add owner/repo" form —
// no --skill, so no name to look up. The npx tool prompts interactively
// here; skm is non-interactive, so zero matches is an error and several
// matches list themselves and ask for --skill.
func findSoleSkillDirectory(root string) (string, error) {
	matches, err := skillMarkerDirectories(root)
	if err != nil {
		return "", err
	}
	switch len(matches) {
	case 0:
		return "", errors.New("no skill or command directory found in this repository")
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("this repository ships several skills (%s); pass --skill <name> to pick one",
			relDirectoryList(root, matches))
	}
}

// skillMarkerDirectories walks root and returns every directory carrying a
// skill or command marker, skipping .git.
func skillMarkerDirectories(root string) ([]string, error) {
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
		if dal.PathExists(filepath.Join(path, "SKILL.md")) || dal.PathExists(filepath.Join(path, "COMMAND.md")) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("search for skills in %q: %w", root, err)
	}
	return matches, nil
}

// relDirectoryList renders match paths relative to the clone root — the form
// a user can act on.
func relDirectoryList(root string, paths []string) string {
	rel := make([]string, len(paths))
	for i, p := range paths {
		r, _ := filepath.Rel(root, p)
		rel[i] = r
	}
	return strings.Join(rel, ", ")
}
