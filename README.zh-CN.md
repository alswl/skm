# skm

**本地优先的 AI 编程技能管理：一个个人技能库，按需安装到各个工具。**

[English](README.md) | 简体中文

[![CI](https://github.com/alswl/skm/actions/workflows/ci.yml/badge.svg)](https://github.com/alswl/skm/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Release](https://img.shields.io/github/v/release/alswl/skm?include_prereleases&sort=semver)](https://github.com/alswl/skm/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

Claude Code 有 `~/.claude/skills`，Codex 有 `~/.codex/skills`，Pi 又是自己的一套。每接入一个新的
AI 编程工具,就多一个技能目录，你精心写好的技能最后被复制粘贴进每一个目录——彼此不同步，重复堆积，
悄悄腐烂。

`skm` 用**你自己的、本地优先的技能库**统一管理技能（skills）和命令（commands），并把它们安装到
你用的每一个工具里。已有本地技能可以直接收编；GitHub、GitLab、Skills.sh 和本地文件也都能导入。之后
一键批量更新，或在 TUI 中浏览、在 CI 里脚本化。需要版本管理或同步时，再选择用 Git 管理技能库。

![skm TUI 演示：初始化、从 skills.sh 和 GitHub 导入、发现并收编未托管的 skill、安装、查看详情](docs/assets/demo.gif)

每个工具一列，`✓` 表示已在该工具中安装。核心思路就这么简单。

## 定制社区 Skill

```sh
skm import owner/repo/path/to/skill
skm fork review                  # 复制到 skills-fork/，并切换已有安装
# 直接编辑 skills-fork/<路径>/ 里的文件（`skm info review` 会输出该路径）。
skm update review                # 把你的修改带到新的上游版本上
skm fork review --discard        # 删除 fork，回到上游
```

fork 就是一份普通拷贝：`skills/<路径>/` 仍是上游，`skills-fork/<路径>/` 是你的版本。
拷贝存在时它就是当前使用的版本：安装指向它，`list`、`info`、`status` 也都报告它
（`info` 和 `status --json` 会多一个 `upstream_path`）。这个目录就是全部状态，
没有需要迁移或清理的东西。fork 创建后立刻生效：已有安装在同一次操作里就切过去了。

**更新。** `skm update` 先取得新的上游，再把你的修改逐个文件合并上去
（`git merge-file`，以当前上游为共同基准，所以 `PATH` 里要有 `git`）。你没改的文件取新上游的版本，
只有你改过的文件保留你的版本，两边做了完全相同的修改只保留一份。全部合并成功后，两个目录一起替换，
已有安装随之刷新。出现冲突、某个二进制文件两边都改了、或某次合并超过 30 秒时，skm 会停下并且什么都不改，
错误信息里会写明文件。请修改 fork 后重新 `update`，或用 `fork --replace` 从上游重新开始。
`update --dry-run` 会完整跑一遍合并但不写任何东西；`batch-update` 会把有冲突的 fork 列入 `failed` 并给出原因，
同时继续更新其余条目。

**替换与丢弃。** `fork --replace` 删除现有 fork 并重新复制上游；`fork --discard` 删除 fork 并回到上游。
命令行不会再确认，请先确认 `skills-fork/` 已提交。对有 fork 的 Skill，归档、删除、normalize 和 convert
会被拒绝，需要先 discard。更新中途被中断不会自动恢复：请用 Git 还原这两个目录。

**在 TUI 里**，`F` 给选中的技能创建 fork。被 fork 的条目在状态列和底部摘要里显示 `active+fork`
（按 `?` 有说明）。`x` 菜单提供创建、替换和丢弃，替换和丢弃会先确认。对已有 fork 的条目按 `F`
只会提示去用 `x`，所以你的拷贝不会被误删。`p` 和 `P` 用于更新，冲突原因会显示在状态栏。
修复损坏的安装是 `f`。

将 `skills/` 和 `skills-fork/` 提交到自己的 Git 仓库；skm 不会替你提交。
符号链接安装指向完整目录，因此目标工具会看到 `skills/<路径>/meta.json`。
已知限制：只会带上文件内容的修改（只改权限位不会被带上），含有悬空符号链接的技能无法更新。

编辑导入地址时，静态反显单元标示插入位置，`NO_COLOR` 下也保持可见，不依赖终端原生光标或窗口焦点。
地址完整可见时，方向键只移动插入位置，不移动文本。

## ✨ 你能得到什么

- 📦 **本地优先，归你掌控。** 技能保存在你的个人 Git 仓库里：纯目录加 Markdown，离线可用；像管理
  代码一样提交、分支、评审和备份。
- 📥 **多平台导入，原有技能不浪费。** 可从 GitHub、GitLab、Skills.sh、本地路径或文件导入；还能发现并
  收编已安装在 Claude Code、Codex 等工具中的本地 skills。
- 🔄 **一键自动化批量更新。** `skm batch-update`（TUI 中按 `P`）逐个更新所有可更新的条目，并保留每项
  结果，省去挨个处理的重复劳动。
- 🔁 **写一次，处处安装。** `skm install code-review` 会装进所有接受该类型的工具，不用再维护三份
  互相不同步的拷贝。
- 🔍 **随时知道现状。** 安装状态列和 `skm status` 会告诉你哪里已装、哪里失效、哪里和源头不一致了。
- 🔌 **高度可扩展。** 平台和目标都是配置，不是代码：`skm target add` 可接入新的本地安装位置；provider
  与 target 插件还能对接自定义平台和安装方式，不用等版本发布。

provider 决定资产从哪里来，target 决定它们装到哪里。两者都自带内置实现，也都可以用纯可执行文件
扩展——不需要 Go，不需要重新编译 skm。

| Provider | 带来什么 |
|---|---|
| Local | 本地磁盘上的一个路径 |
| SelfBuild | 你就地编写的一个 skill/command |
| GitHub | 一个仓库、一个子目录，或直接一个文件的 URL——粘贴 `owner/repo`、`owner/repo/path/to/skill`，或地址栏里的 `blob` 链接都行 |
| GitLab | 任意 GitLab 仓库 |
| Skills.sh | Skills.sh 注册表 |
| *自定义* | 任何插件可执行文件能拉取到的东西 |

| Target | 装到哪里 |
|---|---|
| Claude skills | `~/.claude/skills` |
| Claude commands | `~/.claude/commands` |
| Codex | `~/.codex/skills` |
| pi | `~/.pi/agent/skills` |
| dsh (deepseek-harness) | `~/.dsh/skills`（或 `$DSH_HOME`/skills） |
| 共享 agents | `~/.agents/skills`（或 `$DSH_AGENTS_HOME`/skills） |
| *自定义* | 任意路径，通过 `skm target add` 添加 |

## 🏁 快速上手

安装 skm 并刷新当前 shell 的命令查找：

```bash
curl -fsSL https://raw.githubusercontent.com/alswl/skm/master/install.sh | sh
export PATH="${SKM_INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}:$PATH"
hash -r
```

skm 默认安装到 `~/.local/bin`（可用 `SKM_INSTALL_DIR` 覆盖）。

安装完成后，可选择一个技能集合试用部署。下面仅以 Matt Pocock 的 skills 为例，
你可以换成自己的来源和目标工具：

```bash
skm init ./skm
skm deploy --repo https://github.com/mattpocock/skills.git --root ./skm --target codex,claude
```

deploy 默认使用已初始化的仓库，
可从当前目录向上查找，也可通过 `--root` 指定。`--into` 可显式创建新库，`--no-repo` 可直接复制到目标。
详见 [deploy 文档](docs/cli/skm_deploy.md)。

## 🚀 首次运行

让 skm 指向你的技能仓库，然后直接运行 `skm`——这是主要的使用方式：

```bash
skm init ~/skills && cd ~/skills
skm
```

你会看到一个可搜索的目录列表，每个工具一列安装状态，还有条目详情、目标编辑器（`t`）、以及后台任务
中心（`J`）——长时间运行的任务在这里汇报进度，你可以继续浏览别的内容。

不用从空仓库开始——你的 `~/.claude/skills`、`~/.codex/skills` 之类的地方大概率已经躺着一些技能了。
按 `o` 发现它们，`enter` 收编你想要的进仓库。

| 按键 | 作用 |
|---|---|
| `j` / `k` | 上下移动 · `/` 搜索 · `enter` 详情 |
| `i` | 对选中条目执行安装/卸载 · `f` 修复损坏的安装 |
| `F` | fork 选中的技能（替换或丢弃在 `x` 菜单里） |
| `m` | 导入 · `p` 更新 · `P` 全部更新 · `a` 归档 · `d` 删除 |
| `o` | 发现未受管理的技能 · `c` 收编进仓库 |
| `x` | 当前条目的全部操作 |
| `t` | 目标管理 · `J` 任务中心 · `?` 完整按键表 |

底部状态栏始终显示*此刻*能做什么——不可用的操作会变暗，按下去会告诉你原因，而不是悄无声息地什么
都不做。破坏性操作的确认提示会先说清楚后果。

### 重名条目的寻址方式

条目在仓库中的相对路径就是它的身份。如果两个活跃条目重名，`delete`、`archive`、`install`、`update`
等命令会拒绝裸名字，并列出候选路径。用其中一个路径重新执行即可，例如
`skm delete skills/local/mf-cli --force`。TUI 对选中行始终使用这个完整路径。

想用脚本？每个操作都有对应的 CLI 命令：

```bash
skm discover
skm adopt ~/.codex/skills/review
skm import ./my-skill --kind skill
skm install code-review --target codex
skm list
skm status code-review
skm backup create
skm backup restore --force
```

`deploy`、`install` 和 `uninstall` 的 `--target` 支持具体目标名或平台名。
例如 `--target claude` 会为技能选择 `claude-skills`，为命令选择 `claude-commands`。
具体目标名优先；平台下有多个同类型目标时，需要明确指定目标名。同一目标被重复指定时只执行一次。

`backup create` 记录当前有哪些条目、各自装在哪些目标上，`backup restore` 把它们按记录重新装回去。
它不保存文件内容——内容以你的仓库为准——`--force` 允许重装时覆盖当前占用安装路径的文件。

然后对这个仓库执行 `git init && git commit`，大功告成——你的技能现在被版本化、可迁移，并且已经装
进了每一个工具。

## ⚙️ 配置

配置项的优先级是 **命令行参数 > 环境变量 > 配置文件 > 默认值**，所以写进文件的任何设置，都仍然可以
为单次执行临时覆盖。

```yaml
# ~/.config/skm/config.yaml —— 可选；不写这个文件，一切行为不变
root: ~/skills           # 等同于 --root；省略则从当前目录向上查找
plugin_dirs:             # 在默认的 ~/.config/skm/plugins 之后扫描
  - ~/work/skm-plugins
targets:                 # 自定义 target，以及对内置 target 的覆盖
  - name: my-tool
    path: ~/.mytool/skills
    accepts: [skill]
    strategies:
      skill: skill-symlink
```

| 配置项 | 参数 | 环境变量 | 说明 |
| --- | --- | --- | --- |
| 仓库根目录 | `--root` | `SKM_ROOT` | 未设置时从当前目录向上查找 |
| 额外插件目录 | —— | `SKM_PLUGINS_DIR` | 系统路径列表（`:` 分隔）；会替换配置文件里的列表 |
| 配置目录 | `--config` | `XDG_CONFIG_HOME` | 存放 `config.yaml`；默认 `~/.config/skm` |

`--config` 刻意只做命令行参数：配置文件就在配置目录*里面*，因此它无法反过来决定这个目录。

## 🔌 插件

自定义 provider 和 target 以插件形式和内置实现放在一起——一个坏掉、卡住或运行缓慢的插件是被隔离
的，不会拖垮 skm，也不会阻塞其他插件：

```text
~/.config/skm/plugins/
├── providers/   # 资产从哪里来
└── targets/     # 怎么安装资产
```

不用手工 symlink——`add` 是软链，改了你自己 checkout 里的插件立刻生效：

```bash
skm plugin add ~/ws/skills/skm/plugins/providers/ali-skills   # kind 从 providers/ 推断
skm plugin add ~/bin/my-target --kind target --name acme-target
skm plugin list && skm plugin remove acme-target
```

对于常规 skill 目录，不需要 target plugin：运行
`scripts/skm-targets-bootstrap.sh my-tool "$MY_SKILLS_DIR"`，即可注册共享该目录的 skill 和
command target。只有安装布局或诊断需要自定义行为时才使用 target plugin。

```bash
skm target add --name my-tool --platform mytool --path ~/.mytool/skills \
  --accepts skill --strategy skill=skill-symlink
skm provider list && skm provider validate
skm target plugin list
```

协议细节、错误码和可用的模板：[docs/plugins/README.md](docs/plugins/README.md)。

## 📚 文档与开发

完整命令参考：[docs/cli](docs/cli/)——或者对任何命令执行 `skm <command> --help`。想让 coding agent
来用 skm：[skill/SKILL.md](skill/SKILL.md) 就是为此准备的技能，`skm import ./skill` 装上即可。
想从源码构建：

```bash
git clone git@github.com:alswl/skm.git && cd skm && make build && make install
```

```bash
make build && make test && make lint
```

## License

MIT © Jingchao —— 详见 [LICENSE](LICENSE)。
