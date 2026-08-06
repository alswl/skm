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

```
cmd/skm     → pkg/services → pkg/managers → pkg/dal → {pkg/config, pkg/common}
pkg/tui → pkg/services   (UI adapter; never writes files directly)
```

- **cmd/skm**: one file per command, self-registered in `init()`; only parses
  flags/args and calls `services`. `main.go` only calls `os.Exit(Execute())`.
- **pkg/services**: single orchestration entry shared by CLI and TUI; every
  write path flows through it under lock + rollbackable transaction.
- **pkg/managers**: repository (scan/import/update/lifecycle/convert/discover),
  installer (symlinks/adapters/install-state), providers (registry, Local,
  GitHub, subprocess plugins).
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
