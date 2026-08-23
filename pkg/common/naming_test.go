package common

import "testing"

func TestIsKebabCase(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"my-skill", true},
		{"skill", true},
		{"skill-2", true},
		{"2fa", true},
		{"deepseek-harness-target", true},
		{"My Skill", false},
		{"MySkill", false},
		{"my_Skill", false},
		{"my skill", false},
		{"my.skill", false},
		{"my--skill", false},
		{"-my-skill", false},
		{"my-skill-", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsKebabCase(c.name); got != c.want {
			t.Errorf("IsKebabCase(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}
