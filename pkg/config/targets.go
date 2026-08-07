package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alswl/skm/skm/pkg/common"
)

// InvalidTarget is a targets.json entry that could not be interpreted,
// reported individually rather than discarding the whole config
// (002-open-provider-target FR-016, research R6).
type InvalidTarget struct {
	Raw    json.RawMessage `json:"raw"`
	Reason string          `json:"reason"`
}

// ParseTargets validates/migrates each entry in a targets.json document
// independently: a v2 entry (Accepts set) is validated as-is; a v1 entry
// (legacy Kind only) is migrated per common.InstallTarget's
// EffectiveAccepts/EffectiveStrategy mapping so it round-trips as full v2
// shape; anything else is collected as invalid without blocking the rest
// (target-config.md).
func ParseTargets(data []byte) (valid []common.InstallTarget, invalid []InvalidTarget, err error) {
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return nil, nil, err
	}
	for _, raw := range raws {
		var t common.InstallTarget
		if err := json.Unmarshal(raw, &t); err != nil {
			invalid = append(invalid, InvalidTarget{Raw: raw, Reason: err.Error()})
			continue
		}
		t = expandTarget(t)
		if len(t.Accepts) == 0 {
			// No v2 shape given: migrate from the legacy Kind field.
			accepts, strategies := legacyDefaultsFor(t.Kind)
			if accepts == nil {
				invalid = append(invalid, InvalidTarget{Raw: raw, Reason: fmt.Sprintf("no accepts/strategies and unrecognized legacy kind %q", t.Kind)})
				continue
			}
			t.Accepts, t.Strategies = accepts, strategies
		}
		if reason := ValidateTarget(t); reason != "" {
			invalid = append(invalid, InvalidTarget{Raw: raw, Reason: reason})
			continue
		}
		valid = append(valid, t)
	}
	return valid, invalid, nil
}

// legacyDefaultsFor exposes common.InstallTarget's legacy-kind mapping for
// direct use during migration (the same table EffectiveAccepts/
// EffectiveStrategy derive from, so a migrated entry and a legacy-Kind-only
// entry always resolve identically).
func legacyDefaultsFor(kind common.EntryKind) ([]common.EntryKind, map[common.EntryKind]common.InstallStrategy) {
	t := common.InstallTarget{Kind: kind}
	accepts := t.EffectiveAccepts()
	if accepts == nil {
		return nil, nil
	}
	strategies := make(map[common.EntryKind]common.InstallStrategy, len(accepts))
	for _, k := range accepts {
		s, _ := t.EffectiveStrategy(k)
		strategies[k] = s
	}
	return accepts, strategies
}

// ValidateTarget reports why t is invalid (target-config.md), or "" when
// valid: name/platform/path non-empty, name unique is checked by the caller,
// accepts is a non-empty subset of {skill,command}, and every accepted kind
// has a kind-compatible strategy.
func ValidateTarget(t common.InstallTarget) string {
	if t.Name == "" {
		return "name must be non-empty"
	}
	if t.Path == "" {
		return "path must be non-empty"
	}
	if len(t.Accepts) == 0 {
		return "accepts must be non-empty"
	}
	for _, k := range t.Accepts {
		if k != common.KindSkill && k != common.KindCommand {
			return fmt.Sprintf("accepts contains unknown kind %q", k)
		}
		strategy, ok := t.Strategies[k]
		if !ok {
			return fmt.Sprintf("accepted kind %q has no declared strategy", k)
		}
		if !strategy.CompatibleWith(k) {
			return fmt.Sprintf("strategy %q is not compatible with kind %q", strategy, k)
		}
	}
	return ""
}

// AddTarget validates t, rejects a duplicate name (including a built-in's
// name — customize a built-in via UpdateTarget instead), and appends it to
// configDir's targets.json.
func AddTarget(configDir string, t common.InstallTarget) (common.InstallTarget, error) {
	t = expandTarget(t)
	if reason := ValidateTarget(t); reason != "" {
		return common.InstallTarget{}, fmt.Errorf("target add: %s", reason)
	}
	for _, d := range defaultTargets() {
		if d.Name == t.Name {
			return common.InstallTarget{}, fmt.Errorf("target add: %q already exists; use 'skm target update %q' to customize", t.Name, t.Name)
		}
	}
	targets, _ := loadTargetsRaw(configDir)
	for _, existing := range targets {
		if existing.Name == t.Name {
			return common.InstallTarget{}, fmt.Errorf("target add: %q already exists", t.Name)
		}
	}
	targets = append(targets, t)
	if err := writeTargets(configDir, targets); err != nil {
		return common.InstallTarget{}, err
	}
	return t, nil
}

// UpdateTarget replaces the named target's fields and re-validates it. When
// name matches a built-in that has no user entry yet, a new user-owned
// override entry is seeded from the built-in and inserted into
// targets.json, so future loads merge it in place of the built-in
// (mergeWithBuiltins).
func UpdateTarget(configDir, name string, apply func(*common.InstallTarget)) (common.InstallTarget, error) {
	targets, _ := loadTargetsRaw(configDir)
	for i, t := range targets {
		if t.Name != name {
			continue
		}
		apply(&t)
		t = expandTarget(t)
		if reason := ValidateTarget(t); reason != "" {
			return common.InstallTarget{}, fmt.Errorf("target update: %s", reason)
		}
		targets[i] = t
		if err := writeTargets(configDir, targets); err != nil {
			return common.InstallTarget{}, err
		}
		return t, nil
	}
	for _, d := range defaultTargets() {
		if d.Name != name {
			continue
		}
		apply(&d)
		d = expandTarget(d)
		if reason := ValidateTarget(d); reason != "" {
			return common.InstallTarget{}, fmt.Errorf("target update: %s", reason)
		}
		targets = append(targets, d)
		if err := writeTargets(configDir, targets); err != nil {
			return common.InstallTarget{}, err
		}
		return d, nil
	}
	return common.InstallTarget{}, fmt.Errorf("target update: %q not found", name)
}

// RemoveTarget deletes the named target. Assets already installed through it
// are left untouched (FR-018); removal only stops future installs. A
// built-in with no user override entry can't be removed — its name is
// always present via mergeWithBuiltins, so deleting it here would have no
// effect; the caller must customize it (UpdateTarget) first if they want it
// gone. A user's override entry for a built-in can be removed as usual: the
// built-in default reappears on the next load.
func RemoveTarget(configDir, name string) error {
	targets, _ := loadTargetsRaw(configDir)
	out := make([]common.InstallTarget, 0, len(targets))
	found := false
	for _, t := range targets {
		if t.Name == name {
			found = true
			continue
		}
		out = append(out, t)
	}
	if !found {
		for _, d := range defaultTargets() {
			if d.Name == name {
				return fmt.Errorf("cannot remove built-in target %q; use 'skm target update' to customize", name)
			}
		}
		return fmt.Errorf("target remove: %q not found", name)
	}
	return writeTargets(configDir, out)
}

// loadTargetsRaw reads targets.json without falling back to defaults, for
// the add/update/remove write path (an empty/missing file is an empty list).
func loadTargetsRaw(configDir string) ([]common.InstallTarget, []InvalidTarget) {
	data, err := os.ReadFile(filepath.Join(configDir, targetsFileName))
	if err != nil {
		return nil, nil
	}
	valid, invalid, err := ParseTargets(data)
	if err != nil {
		return nil, nil
	}
	return valid, invalid
}

// writeTargets persists targets as the v2 shape (a v1 file is upgraded on
// its next write).
func writeTargets(configDir string, targets []common.InstallTarget) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("write targets.json: %w", err)
	}
	data, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		return fmt.Errorf("write targets.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, targetsFileName), data, 0o644); err != nil {
		return fmt.Errorf("write targets.json: %w", err)
	}
	return nil
}

// PathState reports whether path is usable: writable (exists and a
// directory, or absent but its parent is writable), missing, or
// not_writable.
func PathState(path string) string {
	if fi, err := os.Stat(path); err == nil {
		if !fi.IsDir() {
			return "not_writable"
		}
		return "writable"
	}
	parent := filepath.Dir(path)
	if fi, err := os.Stat(parent); err == nil && fi.IsDir() {
		return "missing"
	}
	return "not_writable"
}
