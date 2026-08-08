# skm — Local AI Coding Skill & Command Manager

`skm` manages a local repository of AI Coding **skills** and **commands**:
browse, search, import, install, update, archive, convert, adopt external
assets, and replicate them across machines.

This is the **Go rewrite** of the tool. It is a single static binary with a
Bubble Tea TUI (launched with no subcommand) and a scriptable CLI with stable
JSON output (launched when the first positional argument is a known
subcommand). Observable behavior is defined by `docs/req.md` and formalized in
`specs/001-skmgr-rewrite/`.

## Build

Requires Go 1.22+ and `git` on PATH.

```bash
make build                 # builds ./bin/skm via alswl/makefile-go; or: go build -o skm ./cmd/skm
./bin/skm version          # version/commit/date injected via pkg/version ldflags
```

## Run the TUI

```bash
# auto-discovers the repository by walking upward for skills/ or commands/
./bin/skm

# explicit repository root and config dir
./bin/skm --root /work/skills-repo --config /work/skm-config
```

`--root` accepts either the repo root or its `skills/` directory. Default
config dir is `~/.config/skm`.

## Common CLI

```bash
./bin/skm list --json
./bin/skm import ./my-skill --kind skill --force
./bin/skm install my-skill --target codex --json
./bin/skm update review
./bin/skm batch-update --json
./bin/skm verify repo --json
./bin/skm discover --source ~/.codex/skills --json
./bin/skm deploy --repo git@host:team/skills.git --target codex --only review,release
./bin/skm export
```

- `--json` prints one JSON object to stdout; `--timing` prints only to stderr.
- Write commands accept `--dry-run` (no writes) and `--force` (authorize
  overwrite/delete).
- Exit codes: `0` success/warnings, `1` object problem, `2` argument/tool/
  provider/execution error.

## Providers and Targets

Acquisition sources (**Providers**) and install destinations (**Targets**) are both open,
user-configurable capabilities — not fixed to the built-in Local/GitHub sources or the
built-in Claude/Codex/pi targets.

```bash
./bin/skm provider list --json                 # built-ins (local, github, gitlab,
./bin/skm provider validate --json             #   skills-sh) + any plugins

./bin/skm target list --json
./bin/skm target add --name my-tool --platform mytool --path ~/.mytool/skills \
    --accepts skill,command \
    --strategy skill=skill-symlink --strategy command=command-adapter --json
./bin/skm target validate my-tool --json
./bin/skm target update --name my-tool --path ~/.mytool/skills2 --json
./bin/skm target remove --name my-tool --json
```

A Target declares its platform, path, accepted kinds, and per-kind install strategy —
install/uninstall dispatch on that declaration, never on a hardcoded tool name. The TUI
exposes the same operations behind the `t` key (add `a`, edit path `enter`, remove `d`).

### Configuring Targets directly (`~/.config/skm/targets.json`)

The commands above are the safe way to edit it, but the file itself is plain JSON — useful
to see (or hand-edit, or check into dotfiles) directly. A minimal file with the four
built-ins plus one custom tool:

```jsonc
[
  { "name": "claude-skills", "platform": "claude", "path": "~/.claude/skills",
    "accepts": ["skill"], "strategies": { "skill": "skill-symlink" }, "builtin": true },
  { "name": "claude-commands", "platform": "claude", "path": "~/.claude/commands",
    "accepts": ["command"], "strategies": { "command": "command-marker" }, "builtin": true },
  { "name": "codex", "platform": "codex", "path": "~/.codex/skills",
    "accepts": ["skill", "command"],
    "strategies": { "skill": "skill-symlink", "command": "command-adapter" }, "builtin": true },

  // A custom tool: its own path, only skills, symlink strategy.
  { "name": "my-tool", "platform": "mytool", "path": "~/.mytool/skills",
    "accepts": ["skill"], "strategies": { "skill": "skill-symlink" }, "builtin": false }
]
```

Rules: `accepts` is a subset of `["skill", "command"]`; each accepted kind needs a
kind-compatible `strategies` entry (`skill` → `skill-symlink`; `command` →
`command-marker` or `command-adapter`). The four built-ins are always present in `target
list` and the install flow, whether or not `targets.json` exists or has entries of its own —
`target add`ing a custom tool never hides them. To customize a built-in (a different path,
say), use `target update`, which writes your override into `targets.json`; `target add`
rejects a name that collides with a built-in, since there's nothing to add — use `target
update` instead. `target remove` on an unmodified built-in is rejected (there'd be nothing to
remove); once you've overridden one via `target update`, `target remove` deletes that
override and the built-in default reappears on the next load. An old skmgr `targets.json`
(or one found in the legacy `~/.config/skill-manager/`) is read and migrated automatically —
no manual edits needed.

### Configuring a Provider (plugin)

Providers are discovered from `~/.config/skm/plugins/` (or any directory listed in
`SKM_PLUGINS_DIR`, `:`/`;`-separated). Each entry is a single executable — any language,
as long as it speaks the line-based JSON protocol. A minimal working one:

```sh
#!/bin/sh
# ~/.config/skm/plugins/acme  (chmod +x)
IFS= read -r req
case "$req" in
  *'"action":"id"'*)         printf '{"id":"acme"}\n' ;;
  *'"action":"label"'*)      printf '{"label":"Acme Internal Skills"}\n' ;;
  *'"action":"capability"'*) printf '{"description":"Fetches acme://<repo> from our internal git host"}\n' ;;
  *'"action":"can_handle"'*) case "$req" in *'"address":"acme:'*) printf '{"result":true}\n';; *) printf '{"result":false}\n';; esac ;;
  *'"action":"fetch"'*)
    addr=$(printf '%s' "$req" | sed -n 's/.*"address":"acme:\/\/\([^"]*\)".*/\1/p')
    dir=$(mktemp -d)
    git clone --depth 1 "https://git.acme.internal/${addr}.git" "$dir" >/dev/null 2>&1 \
      && printf '{"path":"%s"}\n' "$dir" \
      || printf '{"error":{"code":"fetch_failed","message":"clone failed"}}\n'
    ;;
  *) printf '{"error":{"code":"protocol_error","message":"unknown action"}}\n' ;;
esac
```

```bash
chmod +x ~/.config/skm/plugins/acme
./bin/skm provider list --json                  # "acme" now appears, loaded:true
./bin/skm import "acme://team/skills-repo"       # routed to it automatically
```

Note skm sends the request **without** a trailing newline, so don't use `set -e` in a shell
plugin (`read` returns non-zero at EOF even though it populates the variable correctly — see
[`docs/plugins/README.md`](docs/plugins/README.md) for the full protocol, error codes, and a
copy-pasteable [reference template](docs/plugins/template/) that already handles this).

## Cross-machine deploy

```bash
# Machine A: install what you need, then:
./bin/skm export        # prints a quote-safe `skm deploy …` (nothing if nothing installed)

# Machine B: configure same-named targets, then run the emitted command, e.g.:
./bin/skm deploy --repo git@host:team/skills.git --target codex --only review,release
./bin/skm verify repo --json
```

## Test

```bash
make test                           # unit + golden JSON + filesystem-safety
go test ./... -run Perf -count=1    # perf regression against testdata/perf/baselines
make lint                           # golangci-lint + go vet
```

Golden JSON fixtures live under `testdata/golden/`; filesystem-safety tests use
temp dirs and never touch your real config or targets.

## Architecture

Core domain concepts (`Entry`, `EntryKind`, `Provider`, `Target`, `InstallStrategy`, `Origin`,
`Status`, `InstallState`, `InstallReport`) have one canonical definition, relationship model, and
ownership map in the Concept Reference: `specs/003-engineering-optimization/data-model.md`.

```
cmd/skm     → pkg/services → pkg/managers → pkg/dal → {pkg/config, pkg/common}
pkg/tui → pkg/services   (UI adapter; never writes files directly)
```

- **cmd/skm**: one file per command, self-registered in `init()`; only parses
  flags/args and calls `services`. `main.go` only calls `os.Exit(Execute())`.
- **pkg/services**: single orchestration entry shared by CLI and TUI; every
  write path flows through it under lock + rollbackable transaction.
- **pkg/managers**: repository (scan/import/update/lifecycle/convert/discover),
  installer (strategy-dispatched: symlinks/markers/adapters, install-state),
  providers (registry, Local, GitHub/GitLab, Skills.sh, subprocess
  plugins).
- **pkg/dal**: repository lock, `FileTransaction` (staging + rollback),
  frontmatter/meta.json/targets.json IO, content hashing.
- **pkg/tui**: Bubble Tea list/detail/search; long operations run on a
  FIFO single-concurrency job queue with a stale-result guard.

## Documented divergences (versioned per SC-001)

The rewrite preserves the observable behavior in `docs/req.md`. These
divergences are intentional and caused by the Python→Go target:

| Divergence | Why |
|---|---|
| Distribution is a single Go binary; `export` emits `skm deploy …`, not `uvx … skmgr deploy` | `uvx`/`pip` are Python-only |
| Provider plugins load via a **subprocess JSON protocol** (executables in the plugin dir) instead of Python entry points + `.py` files | Go cannot import `.py`; subprocess keeps failure isolation |
| Config/plugin paths use the literal `~/.config/skm` and `~/.config/skm/plugins` rather than `os.UserConfigDir()` (XDG) | `docs/req.md` fixes these paths; `os.UserConfigDir()` differs on macOS |
| Golden JSON fixtures are reconstructed from the CLI JSON field contract | the Python fixtures are not present in this repository |
