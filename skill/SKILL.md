---
name: skm
description: >-
  Manage AI coding skills and commands with the skm CLI: one local Git repo of
  skills/commands, installed into Claude Code, Codex, pi, dsh and any custom
  target. Use when asked to import/adopt a skill, install or uninstall one
  somewhere, update or verify the repo, inspect what is installed where, add an
  install target, or install a provider/target plugin. Also use when the user
  says skm, "my skills repo", "装到 claude/codex", "同步技能".
---

# skm

skm keeps skills and commands in **one local Git repo** and installs them into every
tool. Two axes: **providers** say where an asset came from; **targets** say where it
gets installed. `skm --help` and `docs/cli/` have the full surface — this file is the
working model plus the commands worth reaching for.

## Before anything else

- Every command takes `--json`. **Always pass it when you're going to parse the
  output**; the human tables are not a stable contract.
- Exit codes: `0` ok, `1` problem with the object you asked about, `2` argument /
  tool / provider error. Don't infer success from stdout.
- The repo root is discovered upward from cwd (a `skills/` or `commands/` dir marks
  it); override with `--root`, or `SKM_ROOT`. If a command says "no repository
  found", you are outside the repo — pass `--root`, don't `skm init` a new one.
- `--dry-run` reports intended actions and writes nothing. Use it first for anything
  that touches target directories.
- Destructive commands (`delete`, `delete-external`) require `--force` by design.
  Ask the user before reaching for them.

## The loop that covers most work

```bash
skm list --json                     # the catalog: every entry, with install columns
skm status <name> --json            # one entry across targets: installed/dangling/drifted
skm install <name> [--target t]     # default: every kind-matching target
skm uninstall <name> [--target t]   # removes managed installs only, never user files
skm verify --json                   # whole-repo consistency
```

Bringing assets in:

```bash
skm import ./my-skill --kind skill          # local path
skm import owner/repo/path/to/skill --json  # GitHub shorthand, subdir, or a blob URL
skm import - < sources.txt                  # newline-separated list on stdin
skm discover --json && skm adopt ~/.codex/skills/review   # take over an unmanaged install
skm update <name> / skm batch-update        # refresh from the recorded origin
```

Entries are plain directories with `SKILL.md`; editing files in the repo is normal
and expected. Never edit an *installed* copy in a target — it is a symlink or a
generated adapter, and drift is what `skm status` will report.

## Targets

A target is a destination path plus the kinds it accepts and the strategy per kind.
Built-ins (Claude skills/commands, Codex, pi, dsh, shared agents) are always present;
add your own instead of copying files around:

```bash
skm target list --json
skm target add --name my-tool --platform mytool --path ~/.mytool/skills \
  --accepts skill --strategy skill=skill-symlink
skm target validate --json
```

## Plugins

Providers and targets are extensible with plain executables — no Go, no rebuild. A
plugin is one executable file speaking line-delimited JSON on stdin/stdout
(`docs/plugins/README.md` has the protocol and a template).

```bash
skm plugin add ~/ws/skills/skm/plugins/providers/ali-skills  # kind read from providers/
skm plugin add ~/bin/my-target --kind target --name codefuse # otherwise say which kind
skm plugin list --json                                        # incl. links whose source is gone
skm plugin remove codefuse [--kind target]
```

`add` **links** the executable into `~/.config/skm/plugins/<kind>s/`, so edits in the
user's own checkout take effect on the next command; `remove` unlinks and never
deletes their file. Loading is isolated: a broken, slow or hung plugin is skipped
with a reason in `skm provider list` / `skm target plugin list`, and never takes skm
down.

## Sharing a setup

```bash
skm share create --json         # source addresses only, never raw content
skm share apply <payload>
skm export                      # a quote-safe `skm deploy` line for what's installed
skm deploy <repo-url>           # clone/pull and batch-install on another machine
```

## Gotchas

- The plugin directory follows `$XDG_CONFIG_HOME`/`$HOME` (`~/.config/skm/plugins`),
  **not** `--config`. `SKM_PLUGINS_DIR` adds more scan dirs; skm only writes the first.
- `skm uninstall` and `skm plugin remove` are deliberately narrow: they remove what
  skm created. Anything the user wrote stays, and skm says so rather than guessing.
- Run `skm` with no subcommand for the TUI; that is the primary human interface, so
  prefer suggesting it over scripting a long sequence of one-off commands.
