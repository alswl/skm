# skm

**One repo for your AI coding skills. Installed everywhere, in sync.**

English | [简体中文](README.zh-CN.md)

[![CI](https://github.com/alswl/skm/actions/workflows/ci.yml/badge.svg)](https://github.com/alswl/skm/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Claude Code has `~/.claude/skills`. Codex has `~/.codex/skills`. Pi has its own. Every new agent
brings another directory, and your carefully written skills end up copy-pasted into all of them —
drifting, duplicating, and quietly rotting.

`skm` keeps **one git-friendly repository** of skills and commands and installs them into every tool
you use. Import from a local path or a git URL, install everywhere with one command, update them all
later. Browse it in a TUI, or script it in CI.

```
┌─ skm · ~/skills ─────────────────────────────────────────────────────────────────────────────────┐
│ 0*  1L                                                                                           │
│       name                     kind    version  status       Claude Claude* Codex Pi             │
│  ▶ 📂 changelog                skill   —        active         ✓                                 │
│    📂 code-review              skill   —        active         ✓              ✓                  │
│    📂 pr-triage                skill   —        active                                           │
│                                                                                                  │
│                                                                                                  │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│1/1 · local · changelog · skill · active                                                          │
├──────────────────────────────────────────────────────────────────────────────────────────────────┤
│[j/k] up/down  [/] search  [i] installs  [m] import  [c] claim  [x] actions  [q] quit             │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
```

One column per tool. A `✓` means installed there. That's the whole idea.

## What you get

- **Write once, install everywhere.** `skm install code-review` puts it in every tool that accepts
  skills. No more three copies drifting apart.
- **Your skills become a git repo.** Plain directories and markdown — commit, branch, review, share
  with your team like any other code.
- **Always know where things stand.** The install columns and `skm status` tell you what's installed
  where, what's dangling, what drifted.
- **Safe by default.** `--dry-run` shows the plan before touching anything, destructive actions
  require `--force`, and `uninstall` only removes what skm installed — files you wrote by hand are
  never touched.
- **New tool tomorrow? Add it in one command.** Tools are configuration, not code. `skm target add`
  teaches skm a new agent; no waiting for a release.
- **Explore in the TUI, automate with the CLI.** Both do the same things, so today's experiment is
  tomorrow's script. Every command speaks `--json`.
- **Move machines in two commands.** `skm export` on the old one, `skm deploy` on the new one.

## Install

Requires Go 1.26+ and `git` on your `PATH`.

```bash
git clone git@github.com:alswl/skm.git
cd skm
make build       # -> ./bin/skm
make install     # optional: copy onto your PATH
```

## Quick start

```bash
# 1. Create a skills repository — plain directories, git-friendly.
skm init ~/skills
cd ~/skills

# 2. Bring assets in, from a local path or a git remote.
skm import ./my-skill --kind skill
skm import git@github.com:org/skills.git

# 3. Install into every tool that accepts this kind — or pick targets.
skm install code-review
skm install code-review --target codex

# 4. See where things stand.
skm list
skm status code-review

# 5. Later: refresh everything that came from somewhere.
skm batch-update
```

Then `git init && git commit` the repository and you're done — your skills are now versioned,
portable, and installed in every tool at once.

Run `skm` with no subcommand to do all of it interactively.

## Everyday workflows

### Try before you commit

Any write operation accepts `--dry-run`, which reports what *would* happen and changes nothing.

```bash
skm install code-review --dry-run
skm batch-update --dry-run
```

### Collect the skills already scattered across your tools

You almost certainly have skills sitting in `~/.claude/skills` or `~/.codex/skills` that skm doesn't
manage yet. Find them, then take them over — or clean them out.

```bash
skm discover                        # what's out there but unmanaged?
skm adopt ~/.codex/skills/review    # move it into the repo, replace it with a managed install
skm delete-external ~/.codex/skills/obsolete --force
```

### Share a team repository

Point your team at one git repo and let everyone install from it.

```bash
skm import git@github.com:team/skills.git     # pull the whole repo in
skm install release-notes --target codex
skm batch-update                              # later: pull everyone's updates in one go
```

### Set up a new machine

```bash
# On the old machine — prints a ready-to-run command reproducing your installs:
skm export

# On the new one:
skm deploy --repo git@github.com:team/skills.git --target codex --only review,release
```

### Curate the repository

```bash
skm info code-review        # metadata, files, frontmatter
skm archive old-skill --force / skm unarchive old-skill
skm to-command review       # a skill and a command are the same content, different layout
skm to-skill review
skm verify                  # whole-repository consistency check
```

### Script it

Every command takes `--json`, so skm composes with `jq`, Makefiles, and CI:

```bash
# Everything not yet installed anywhere
skm list --json | jq -r '.entries[] | select(.installed | not) | .name'

# Install a curated set on a fresh CI runner
for s in code-review changelog; do skm install "$s" --json; done
```

Global flags: `--json` (machine-readable stdout), `--dry-run` (plan only), `--force` (authorize
overwrite/delete), `--root` (pick the repository), `--config` (pick the config dir), `--timing`
(timings on stderr only).

## The TUI

Run `skm` with no arguments. You get a searchable catalog with one install column per tool, entry
detail, a target editor (`t`), and a task center (`J`) where long-running jobs report progress while
you keep browsing.

| Key | Action |
|---|---|
| `j` / `k` | move · `/` search · `enter` detail |
| `i` | install / uninstall for the selected entry |
| `m` | import · `p` update · `P` update all · `a` archive · `d` delete |
| `o` | discover unmanaged skills · `c` claim one into the repo |
| `x` | all actions for the current entry |
| `t` | targets · `J` task center · `?` full key map |

The footer always shows what's available *right now* — unavailable actions are dimmed, and pressing
one tells you why instead of silently doing nothing. Confirmations spell out the consequence before
anything destructive happens.

## Configure your tools

Targets ship preconfigured for Claude skills, Claude commands, Codex, and pi. Adding another agent
is one command:

```bash
skm target add --name my-tool --platform mytool --path ~/.mytool/skills \
  --accepts skill --strategy skill=skill-symlink

skm target list
skm target validate my-tool
skm target update --name my-tool --path ~/.mytool/skills-v2
```

A target is just a path, the kinds it accepts (`skill`, `command`), and an install strategy —
`skill-symlink`, `command-marker`, `command-adapter`, or `plugin:<id>`. Config lives in
`targets.json` under `~/.config/skm` (or `$XDG_CONFIG_HOME/skm`, or `--config`).

## Extend it

Where assets come from (**providers**) and how they get installed (**targets**) are both pluggable
with plain executables — no Go, no rebuilding skm. Built-in providers cover Local, SelfBuild, GitHub,
GitLab, and Skills.sh; drop your own beside them:

```text
~/.config/skm/plugins/
├── providers/   # where assets come from
└── targets/     # how assets get installed
```

A plugin that's broken, slow, or hung is isolated — it never takes skm down or blocks other plugins.
`SKM_PLUGINS_DIR` adds more directories.

```bash
skm provider list && skm provider validate
skm target plugin list
```

Protocol, error codes, and working templates: [docs/plugins/README.md](docs/plugins/README.md).

## Docs & development

Full command reference: [docs/cli](docs/cli/) — or `skm <command> --help` for anything.

```bash
make build && make test && make lint
```

## License

MIT © Jingchao — see [LICENSE](LICENSE).
