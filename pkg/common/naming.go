package common

import "regexp"

// kebabCaseRE matches the Agent-Skills name convention shared by
// deepseek-harness and the cross-agent skills standard
// (006-deepseek-harness-target): lowercase letters/digits, dash-separated,
// no leading or trailing dash.
var kebabCaseRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// IsKebabCase reports whether name conforms to the kebab-case rule a target
// can declare via InstallTarget.NameRule (data-model.md). Skill names that
// fail this rule are not discoverable by deepseek-harness, so targets that
// declare it reject such names at install time (FR-006).
func IsKebabCase(name string) bool {
	return kebabCaseRE.MatchString(name)
}

// NameSatisfies reports whether name conforms to a declared InstallTarget
// name rule. Only "kebab-case" is defined today; an unknown rule returns
// false so a target declaring an unsupported rule rejects loudly instead of
// silently accepting names the rule was meant to bound.
func NameSatisfies(rule, name string) bool {
	switch rule {
	case "kebab-case":
		return IsKebabCase(name)
	}
	return false
}
