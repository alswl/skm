package common

import "testing"

func TestInstallTargetEffectiveAcceptsV2(t *testing.T) {
	target := InstallTarget{Accepts: []EntryKind{KindSkill}}
	if !target.AcceptsKind(KindSkill) {
		t.Errorf("v2 target should accept its declared kind")
	}
	if target.AcceptsKind(KindCommand) {
		t.Errorf("v2 target should not accept an undeclared kind")
	}
}

func TestPluginSkillTargetAdaptsCommandsAsSkills(t *testing.T) {
	target := InstallTarget{
		Accepts: []EntryKind{KindSkill},
		Strategies: map[EntryKind]InstallStrategy{
			KindSkill: PluginStrategy("acme"),
		},
	}
	if !target.AcceptsKind(KindCommand) {
		t.Fatal("a plugin skill target must accept commands through the skill adapter fallback")
	}
	strategy, ok := target.EffectiveStrategy(KindCommand)
	if !ok || strategy != StrategyCommandAdapter {
		t.Fatalf("command fallback = %q,%v, want %q,true", strategy, ok, StrategyCommandAdapter)
	}
}

func TestInstallTargetEffectiveAcceptsLegacyMapping(t *testing.T) {
	cases := []struct {
		kind    EntryKind
		accepts []EntryKind
	}{
		{KindSkill, []EntryKind{KindSkill, KindCommand}},
		{KindCommand, []EntryKind{KindCommand}},
	}
	for _, c := range cases {
		target := InstallTarget{Kind: c.kind}
		got := target.EffectiveAccepts()
		if len(got) != len(c.accepts) {
			t.Fatalf("legacy Kind=%q: got %v, want %v", c.kind, got, c.accepts)
		}
		for i, k := range c.accepts {
			if got[i] != k {
				t.Errorf("legacy Kind=%q: got %v, want %v", c.kind, got, c.accepts)
			}
		}
	}
}

func TestInstallTargetEffectiveStrategyLegacyMapping(t *testing.T) {
	skillTarget := InstallTarget{Kind: KindSkill}
	if s, ok := skillTarget.EffectiveStrategy(KindSkill); !ok || s != StrategySkillSymlink {
		t.Errorf("skill-kind target's own kind: got %q,%v, want %q,true", s, ok, StrategySkillSymlink)
	}
	if s, ok := skillTarget.EffectiveStrategy(KindCommand); !ok || s != StrategyCommandAdapter {
		t.Errorf("skill-kind target accepting a command: got %q,%v, want %q,true", s, ok, StrategyCommandAdapter)
	}

	cmdTarget := InstallTarget{Kind: KindCommand}
	if s, ok := cmdTarget.EffectiveStrategy(KindCommand); !ok || s != StrategyCommandMarker {
		t.Errorf("command-kind target: got %q,%v, want %q,true", s, ok, StrategyCommandMarker)
	}
	if _, ok := cmdTarget.EffectiveStrategy(KindSkill); ok {
		t.Errorf("command-kind target must not accept skill")
	}
}

func TestInstallStrategyCompatibleWith(t *testing.T) {
	if !StrategySkillSymlink.CompatibleWith(KindSkill) {
		t.Errorf("skill-symlink must be compatible with skill")
	}
	if StrategySkillSymlink.CompatibleWith(KindCommand) {
		t.Errorf("skill-symlink must not be compatible with command")
	}
	if !StrategyCommandAdapter.CompatibleWith(KindCommand) {
		t.Errorf("command-adapter must be compatible with command")
	}
	if !PluginStrategy("acme").CompatibleWith(KindSkill) {
		t.Errorf("a plugin strategy is structurally compatible with any kind")
	}
}

func TestInstallStrategyPlugin(t *testing.T) {
	s := PluginStrategy("acme")
	if !s.IsPlugin() {
		t.Errorf("PluginStrategy result must report IsPlugin")
	}
	if got := s.PluginID(); got != "acme" {
		t.Errorf("PluginID() = %q, want %q", got, "acme")
	}
	if StrategySkillSymlink.IsPlugin() {
		t.Errorf("a built-in strategy must not report IsPlugin")
	}
	if got := StrategySkillSymlink.PluginID(); got != "" {
		t.Errorf("PluginID() of a non-plugin strategy = %q, want \"\"", got)
	}
}

func TestEntryValueAccessors(t *testing.T) {
	e := &Entry{Path: "skills/foo", Kind: KindSkill}
	if e.ProviderIDValue() != "" {
		t.Errorf("unset ProviderID should report empty string")
	}
	if e.GroupValue() != "" {
		t.Errorf("unset Group should report empty string")
	}
	if e.VersionValue() != "" {
		t.Errorf("unset Version should report empty string")
	}
	if !e.IsDirectory() {
		t.Errorf("a path without .md suffix should be a directory entry")
	}
	cmdEntry := &Entry{Path: "commands/foo.md", Kind: KindCommand}
	if cmdEntry.IsDirectory() {
		t.Errorf("a single-file command path should not be a directory entry")
	}
	if got := cmdEntry.MarkerPath(); got != "commands/foo.md" {
		t.Errorf("single-file command MarkerPath() = %q, want the file itself", got)
	}
}

func TestEntryKindMarkerFileAndTopDir(t *testing.T) {
	if KindSkill.MarkerFile() != "SKILL.md" || KindSkill.TopDir() != "skills" {
		t.Errorf("KindSkill marker/topdir mismatch: %q/%q", KindSkill.MarkerFile(), KindSkill.TopDir())
	}
	if KindCommand.MarkerFile() != "command.md" || KindCommand.TopDir() != "commands" {
		t.Errorf("KindCommand marker/topdir mismatch: %q/%q", KindCommand.MarkerFile(), KindCommand.TopDir())
	}
}
