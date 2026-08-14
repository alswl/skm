# skm

**Local-first AI coding-skill management: one personal Git repo, installed everywhere.**

English | [简体中文](README.zh-CN.md)

[![CI](https://github.com/alswl/skm/actions/workflows/ci.yml/badge.svg)](https://github.com/alswl/skm/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/release/alswl/skm?include_prereleases&sort=semver)](https://github.com/alswl/skm/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Claude Code has `~/.claude/skills`. Codex has `~/.codex/skills`. Pi has its own. Every new agent
brings another directory, and your carefully written skills end up copy-pasted into all of them —
drifting, duplicating, and quietly rotting.

`skm` keeps **your own, local-first Git repository** of skills and commands and installs them into
every tool you use. Adopt skills you already have locally, or import from GitHub, GitLab, Skills.sh,
and local files. Then batch-update them with one automated command, browse them in a TUI, or script
them in CI.

![skm TUI demo: init, import from skills.sh and GitHub, discover and adopt an unmanaged skill, install, and inspect it](docs/assets/demo.gif)

One column per tool. A `✓` means installed there. That's the whole idea.

## ✨ What you get

- 📦 **Local first, fully yours.** Skills live in your personal Git repo: plain directories and
  Markdown, usable offline and easy to commit, branch, review, and back up like any other code.
- 📥 **Import from the platforms you use.** Bring in skills from GitHub, GitLab, Skills.sh, local
  paths, or files — and discover and adopt skills already installed for Claude Code, Codex, and more.
- 🔄 **Automated batch updates in one command.** `skm update --all` (or `P` in the TUI) updates every
  eligible entry and keeps the result for each one, so there is no repetitive manual cleanup.
- 🔁 **Write once, install everywhere.** `skm install code-review` puts it in every tool that accepts
  skills. No more three copies drifting apart.
- 🔍 **Always know where things stand.** The install columns and `skm status` tell you what's installed
  where, what's dangling, what drifted.
- 🔌 **Built to extend.** Platforms and targets are configuration, not code: `skm target add` connects
  another local destination, while provider and target plugins support custom platforms and install
  behavior — no release required.

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

## 🏁 Quick start

Downloads the release binary for your platform, verifies its checksum, and installs it to
the first writable directory on your `PATH`:

```bash
curl -fsSL https://raw.githubusercontent.com/alswl/skm/master/install.sh | sh
```

## 🚀 First run

Point skm at your skills repo, then just run `skm` — that's the primary way to use it:

```bash
skm init ~/skills && cd ~/skills
skm
```

You get a searchable catalog with one install column per tool, entry detail, a target editor
(`t`), and a task center (`J`) where long-running jobs report progress while you keep browsing.

Don't start from a blank repo — you almost certainly have skills already sitting in
`~/.claude/skills`, `~/.codex/skills`, and the like. Press `o` to discover them, `enter` to adopt
the ones you want into the repo.

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
skm discover
skm adopt ~/.codex/skills/review
skm import ./my-skill --kind skill
skm install code-review --target codex
skm list
skm status code-review
```

Then `git init && git commit` the repository and you're done — your skills are now versioned,
portable, and installed in every tool at once.

## 🔌 Plugins

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

Full command reference: [docs/cli](docs/cli/) — or `skm <command> --help` for anything. Build from
source instead of installing a release:

```bash
git clone git@github.com:alswl/skm.git && cd skm && make build && make install
```

```bash
make build && make test && make lint
```

## License

MIT © Jingchao — see [LICENSE](LICENSE).
