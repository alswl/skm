# makefile-go

A batteries-included Makefile toolkit for Go projects.
Covers build, test, code generation, versioning, containerization.

Do NOT edit any files in this directory.

Synced from https://github.com/alswl/makefile-go.

## Usage

1. Copy this directory into your project's `hack/`
2. Copy `sample.mk` from the repo root as your `Makefile`
3. Replace the `your-` placeholders

Run `make help` to see all available targets.

## Install Target

`make install` installs binaries to PATH. When `INSTALL_DIR` is unset, auto-detects
the first writable directory:

1. `/opt/homebrew/bin` (Apple Silicon Mac)
2. `$HOME/.local/bin`
3. `/usr/local/bin`
4. `$GOPATH/bin`
5. `$HOME/go/bin`

```shell
make install                              # auto-detect
make install INSTALL_DIR=/usr/local/bin   # explicit
```

## CLI Install Script

`hack/install.template.sh` — copy and replace `__GITHUB_REPO__` and `__BINARY_NAME__`.

```shell
curl -sSL https://raw.githubusercontent.com/<user>/<repo>/main/hack/install.sh | sh
```

## Bump Scripts

| Script                      | Purpose                                    |
|-----------------------------|--------------------------------------------|
| `hack/bump.sh`              | Project version bump (`semtag` + `git-cliff`) |
| `hack/bump-sub-mod.sh`     | Sub-module version bump                    |
| `hack/gen-changelog.sh`     | Standalone CHANGELOG generation            |
