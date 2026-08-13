# skm

**One repo for your AI coding skills. Installed everywhere, in sync.**

English | [简体中文](README.zh-CN.md)

[![CI](https://github.com/alswl/skm/actions/workflows/ci.yml/badge.svg)](https://github.com/alswl/skm/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/release/alswl/skm?include_prereleases&sort=semver)](https://github.com/alswl/skm/releases)
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

## 🏁 Quick start

### Install the latest release (macOS & Linux)

Downloads the release binary for your platform, verifies its checksum, and installs it to
the first writable directory on your `PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/alswl/skm/master/install.sh | sh
```

Pin a version or pick the install directory with environment variables:

```bash
SKM_VERSION=v0.1.1 SKM_INSTALL_DIR=$HOME/.local/bin \
  curl -fsSL https://raw.githubusercontent.com/alswl/skm/master/install.sh | sh
```

### Build from source instead

Requires Go 1.26+ and `git` on your `PATH`.

```bash
git clone git@github.com:alswl/skm.git && cd skm && make build && make install
```

Then point skm at your skills repo:

```bash
skm init ~/skills && cd ~/skills
```

Don't start from a blank repo — you almost certainly have skills already sitting in
`~/.claude/skills`, `~/.codex/skills`, and the like. Discover them and adopt the ones you want:

```bash
skm discover                        # what's out there but unmanaged?
skm adopt ~/.codex/skills/review    # pull it into the repo, replace it with a managed install
```

In the TUI that's `o` to discover, then `enter` to adopt.

Run `skm` with no arguments — that's the primary way to use it. A searchable catalog with one
install column per tool, entry detail, a target editor (`t`), and a task center (`J`) where
long-running jobs report progress while you keep browsing.

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

Prefer scripting? Every action has a CLI equivalent:

```bash
skm import ./my-skill --kind skill
skm install code-review --target codex
skm list
skm status code-review
```

Then `git init && git commit` the repository and you're done — your skills are now versioned,
portable, and installed in every tool at once.

## ✨ What you get

- 🔁 **Write once, install everywhere.** `skm install code-review` puts it in every tool that accepts
  skills. No more three copies drifting apart.
- 📦 **Your skills become a git repo.** Plain directories and markdown — commit, branch, review, share
  with your team like any other code.
- 🔍 **Always know where things stand.** The install columns and `skm status` tell you what's installed
  where, what's dangling, what drifted.
- 🔌 **New tool tomorrow? Add it in one command.** Tools are configuration, not code. `skm target add`
  teaches skm a new agent; no waiting for a release.

Providers say where assets come from; targets say where they get installed. Both ship with
built-ins, and both are pluggable with plain executables — no Go, no rebuilding skm.

| Provider | Brings in |
|---|---|
| Local | A path on disk |
| SelfBuild | A skill/command you author in place |
| GitHub | A repo, a subdirectory, or one file's URL — paste `owner/repo`, `owner/repo/path/to/skill`, or a `blob` link straight from the address bar |
| GitLab | Any GitLab repo |
| Skills.sh | The Skills.sh registry |
| *your own* | Anything a plugin executable can fetch |

| Target | Installs into |
|---|---|
| Claude skills | `~/.claude/skills` |
| Claude commands | `~/.claude/commands` |
| Codex | `~/.codex/skills` |
| pi | `~/.pi/agent/skills` |
| *your own* | Any path, via `skm target add` |

Custom providers and targets live beside the built-ins as plugins — a plugin that's broken, slow, or
hung is isolated and never takes skm down or blocks the others:

```text
~/.config/skm/plugins/
├── providers/   # where assets come from
└── targets/     # how assets get installed
```

```bash
skm target add --name my-tool --platform mytool --path ~/.mytool/skills \
  --accepts skill --strategy skill=skill-symlink
skm provider list && skm provider validate
skm target plugin list
```

Protocol, error codes, and working templates: [docs/plugins/README.md](docs/plugins/README.md).

## 📚 Docs & development

Full command reference: [docs/cli](docs/cli/) — or `skm <command> --help` for anything.

```bash
make build && make test && make lint
```

## License

MIT © Jingchao — see [LICENSE](LICENSE).
