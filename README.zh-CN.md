# skm

**AI 编程技能，一个仓库管理，处处同步安装。**

[English](README.md) | 简体中文

[![CI](https://github.com/alswl/skm/actions/workflows/ci.yml/badge.svg)](https://github.com/alswl/skm/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Claude Code 有 `~/.claude/skills`，Codex 有 `~/.codex/skills`，Pi 又是自己的一套。每接入一个新的
AI 编程工具,就多一个技能目录，你精心写好的技能最后被复制粘贴进每一个目录——彼此不同步，重复堆积，
悄悄腐烂。

`skm` 用**一个 git 友好的仓库**统一管理技能（skills）和命令（commands），并把它们安装到你用的每一
个工具里。从本地路径或 git 地址导入，一条命令装到所有工具，之后统一更新。可以在 TUI 里浏览，也可以
在 CI 里脚本化。

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

每个工具一列，`✓` 表示已在该工具中安装。核心思路就这么简单。

## 你能得到什么

- **写一次，处处安装。** `skm install code-review` 会装进所有接受该类型的工具，不用再维护三份互相
  不同步的拷贝。
- **技能变成一个 git 仓库。** 纯目录加 Markdown——像管理代码一样提交、分支、评审、和团队共享。
- **随时知道现状。** 安装状态列和 `skm status` 会告诉你哪里已装、哪里失效、哪里和源头不一致了。
- **默认安全。** `--dry-run` 会在动手前先展示计划；破坏性操作需要显式加 `--force`；`uninstall`
  只会移除 skm 自己装的东西——你手写的文件永远不会被碰。
- **明天来个新工具？一条命令搞定。** 工具是配置，不是代码。`skm target add` 就能让 skm 认识一个
  新的 agent，不用等版本发布。
- **TUI 里探索，CLI 里自动化。** 两者做的是同一件事，今天的手动尝试就是明天的脚本。所有命令都支持
  `--json`。
- **换机器只要两条命令。** 旧机器上 `skm export`，新机器上 `skm deploy`。

## 安装

需要 Go 1.26+ 和 `PATH` 里的 `git`。

```bash
git clone git@github.com:alswl/skm.git
cd skm
make build       # -> ./bin/skm
make install     # 可选：拷贝到 PATH 里
```

## 快速上手

```bash
# 1. 创建一个技能仓库——纯目录，对 git 友好。
skm init ~/skills
cd ~/skills

# 2. 把资产导入进来，可以来自本地路径，也可以来自 git 地址。
skm import ./my-skill --kind skill
skm import git@github.com:org/skills.git

# 3. 装进所有接受这个类型的工具——或者指定目标。
skm install code-review
skm install code-review --target codex

# 4. 查看现状。
skm list
skm status code-review

# 5. 之后：把所有有来源记录的资产统一刷新一遍。
skm batch-update
```

然后对这个仓库执行 `git init && git commit`，大功告成——你的技能现在被版本化、可迁移，并且已经装
进了每一个工具。

不带任何子命令运行 `skm`，就能以交互方式完成以上全部操作。

## 日常工作流

### 先试跑，再落地

所有写操作都支持 `--dry-run`，只报告*将要*发生什么，不改动任何东西。

```bash
skm install code-review --dry-run
skm batch-update --dry-run
```

### 收编散落在各个工具里的技能

你的 `~/.claude/skills` 或 `~/.codex/skills` 里大概率已经躺着一些 skm 还没接管的技能。先找出来，
再决定收编还是清理。

```bash
skm discover                        # 有哪些东西存在，但 skm 还没管？
skm adopt ~/.codex/skills/review    # 挪进仓库，替换成受管理的安装
skm delete-external ~/.codex/skills/obsolete --force
```

### 共享一个团队仓库

让团队都指向同一个 git 仓库，从这里统一安装。

```bash
skm import git@github.com:team/skills.git     # 把整个仓库拉进来
skm install release-notes --target codex
skm batch-update                              # 之后：一条命令拉取团队所有人的更新
```

### 迁移到新机器

```bash
# 在旧机器上——打印出一条能重现当前安装状态的命令：
skm export

# 在新机器上：
skm deploy --repo git@github.com:team/skills.git --target codex --only review,release
```

### 整理仓库

```bash
skm info code-review        # 元数据、文件列表、frontmatter
skm archive old-skill --force / skm unarchive old-skill
skm to-command review       # skill 和 command 本质是同一份内容，只是布局不同
skm to-skill review
skm verify                  # 整个仓库的一致性检查
```

### 脚本化

所有命令都支持 `--json`，方便和 `jq`、Makefile、CI 组合使用：

```bash
# 找出所有还没在任何地方安装的条目
skm list --json | jq -r '.entries[] | select(.installed | not) | .name'

# 在全新的 CI runner 上安装一组指定技能
for s in code-review changelog; do skm install "$s" --json; done
```

全局参数：`--json`（输出机器可读的结果）、`--dry-run`（只出计划）、`--force`（授权覆盖/删除）、
`--root`（指定仓库位置）、`--config`（指定配置目录）、`--timing`（耗时信息只输出到 stderr）。

## TUI

不带参数运行 `skm`。你会看到一个可搜索的目录列表，每个工具一列安装状态，还有条目详情、目标编辑器
（`t`）、以及后台任务中心（`J`）——长时间运行的任务在这里汇报进度，你可以继续浏览别的内容。

| 按键 | 作用 |
|---|---|
| `j` / `k` | 上下移动 · `/` 搜索 · `enter` 详情 |
| `i` | 对选中条目执行安装/卸载 |
| `m` | 导入 · `p` 更新 · `P` 全部更新 · `a` 归档 · `d` 删除 |
| `o` | 发现未受管理的技能 · `c` 收编进仓库 |
| `x` | 当前条目的全部操作 |
| `t` | 目标管理 · `J` 任务中心 · `?` 完整按键表 |

底部状态栏始终显示*此刻*能做什么——不可用的操作会变暗，按下去会告诉你原因，而不是悄无声息地什么
都不做。破坏性操作的确认提示会先说清楚后果。

## 配置你的工具

内置目标已预先配置好 Claude skills、Claude commands、Codex 和 pi。接入另一个 agent 只需一条命令：

```bash
skm target add --name my-tool --platform mytool --path ~/.mytool/skills \
  --accepts skill --strategy skill=skill-symlink

skm target list
skm target validate my-tool
skm target update --name my-tool --path ~/.mytool/skills-v2
```

一个 target 就是一个路径、它接受的类型（`skill`、`command`）、以及一种安装策略——
`skill-symlink`、`command-marker`、`command-adapter`，或 `plugin:<id>`。配置存放在
`~/.config/skm`（或 `$XDG_CONFIG_HOME/skm`，或 `--config` 指定的目录）下的 `targets.json` 里。

## 扩展它

资产从哪里来（**provider**）、怎么安装（**target**）都可以用纯可执行文件扩展——不需要 Go，不需要
重新编译 skm。内置 provider 覆盖 Local、SelfBuild、GitHub、GitLab 和 Skills.sh；把你自己的放在
旁边即可：

```text
~/.config/skm/plugins/
├── providers/   # 资产从哪里来
└── targets/     # 怎么安装资产
```

一个坏掉、卡住或运行缓慢的插件是被隔离的——它不会拖垮 skm，也不会阻塞其他插件。用
`SKM_PLUGINS_DIR` 可以添加更多目录。

```bash
skm provider list && skm provider validate
skm target plugin list
```

协议细节、错误码和可用的模板：[docs/plugins/README.md](docs/plugins/README.md)。

## 文档与开发

完整命令参考：[docs/cli](docs/cli/)——或者对任何命令执行 `skm <command> --help`。

```bash
make build && make test && make lint
```

## 许可证

MIT © Jingchao —— 详见 [LICENSE](LICENSE)。
