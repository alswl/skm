package providers

// NewSkillsSh returns the built-in skills.sh provider (git-backed,
// "skills.sh://<owner>/<repo>"; host overridable via SKM_SKILLS_SH_HOST).
// skills.sh indexes skills living in public GitHub repos, so the default host
// is github.com; the scheme keeps the skills.sh identity while the actual
// clone happens against the (overridable) host.
func NewSkillsSh() Provider {
	return gitBackedProvider{
		id: "skills-sh", label: "Skills.sh", scheme: "skills.sh://",
		envHostVar: "SKM_SKILLS_SH_HOST", defaultHost: "github.com",
	}
}
