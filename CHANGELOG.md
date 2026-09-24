## [0.4.2] - 2026-09-24

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.4.1-dev
- Use the CHANGELOG section as the release description
- Span the release notes from the last published release
## [0.4.1] - 2026-09-24

### 🚀 Features

- Register a target plugin's target on plugin add

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.4.0-dev
- Bump version to v0.4.1
## [0.4.0] - 2026-09-24

### 🚀 Features

- Job spinner and bounded timeouts; update refreshes installs
- Add compact skill share install
- Replace share install with source-address share/apply, add local backup
- Backup and restore

### 🐛 Bug Fixes

- Skip unshareable entries when sharing

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.3.1-dev
- Bump version to v0.4.0
## [0.3.1] - 2026-09-15

### 🚀 Features

- Support bare npx skills add <owner>/<repo> and group skills-sh imports

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.3.0-dev
- Bump version to v0.3.1
## [0.3.0] - 2026-09-10

### 🐛 Bug Fixes

- Reject ambiguous entry references, focus imported skill after rescan

### 🚜 Refactor

- Move targets into config.yaml and fix update provider identity

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.2.0-dev
- Pin Go version and clarify architecture boundary
- Bump version to v0.3.0
## [0.2.0] - 2026-08-25

### 🚀 Features

- Deepseek-harness install targets + services/providers/targets restructure

### 📚 Documentation

- Refine README value proposition

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.1.2-dev
- Bump version to v0.2.0
## [0.1.2] - 2026-08-14

### 🚀 Features

- Recognize skills.sh's npx command and page URL as import addresses
- Preserve external skill directory identity on adopt

### 🐛 Bug Fixes

- Batch update
- Accept www.skills.sh page URLs and use triangle icon for skills-sh provider

### 📚 Documentation

- Add animated TUI demo gif, restructure and re-sync READMEs

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.1.1-dev
- Bump version to v0.1.2
## [0.1.1] - 2026-08-12

### 📚 Documentation

- Highlight providers/targets, add tag-triggered release workflow
- Recommend discover+adopt for quick start, sync zh-CN README

### ⚙️ Miscellaneous Tasks

- Prepare next version v0.1.0-dev
- Bump version to v0.1.1
## [0.1.0] - 2026-08-11

### 🚀 Features

- Rewrite skmgr in Go (001-skmgr-rewrite)
- Open Provider & Target, TUI gap-fill, and post-completion refinements (002)
- Open Provider & Target plugin protocol, install-status columns, TUI polish
- Complete TUI UX and simplify services
- Complete TUI UX review and refinements
- Add CLI parity for TUI operations
- *(engines)* Bootstrap and repair skill repositories

### 🐛 Bug Fixes

- Local using
- Make Codex command adapters discoverable

### 🚜 Refactor

- Engineering-optimization pass (003) — concepts, data-flow, structure

### ⚙️ Miscellaneous Tasks

- Init gitignore
- Bump version to v0.1.0
