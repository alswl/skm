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

// A plugin-backed skill target reaches commands through the command-adapter
// without declaring command in Accepts — the one derivation EffectiveAccepts/
// EffectiveStrategy still make.
func TestInstallTargetPluginSkillTargetAlsoAcceptsCommands(t *testing.T) {
	target := InstallTarget{
		Accepts:    []EntryKind{KindSkill},
		Strategies: map[EntryKind]InstallStrategy{KindSkill: PluginStrategy("acme")},
	}
	accepts := target.EffectiveAccepts()
	if len(accepts) != 2 || accepts[0] != KindSkill || accepts[1] != KindCommand {
		t.Fatalf("plugin skill target accepts: got %v, want [skill command]", accepts)
	}
	if s, ok := target.EffectiveStrategy(KindCommand); !ok || s != StrategyCommandAdapter {
		t.Errorf("plugin skill target's command strategy: got %q,%v, want %q,true", s, ok, StrategyCommandAdapter)
	}
}

// A non-plugin target gets no such derivation: undeclared kinds are refused.
func TestInstallTargetUndeclaredKindIsRefused(t *testing.T) {
	target := InstallTarget{
		Accepts:    []EntryKind{KindSkill},
		Strategies: map[EntryKind]InstallStrategy{KindSkill: StrategySkillSymlink},
	}
	if got := target.EffectiveAccepts(); len(got) != 1 || got[0] != KindSkill {
		t.Fatalf("accepts: got %v, want [skill]", got)
	}
	if _, ok := target.EffectiveStrategy(KindCommand); ok {
		t.Errorf("undeclared command kind must not resolve to a strategy")
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
