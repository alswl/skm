# skm plugins

skm has two independent plugin kinds, both **out-of-process executables you drop into a
directory** — no modifying or rebuilding skm required:

- **Provider plugins** — where skm *acquires* assets from (this document's main section).
- **Target plugins** — how skm *installs* an asset into a tool with no built-in support
  (see [Target plugins](#target-plugins) below).

Each kind is discovered separately, and a broken/slow/hung plugin of either kind is
**isolated**: it never crashes skm and never prevents any other plugin (of either kind, or
a built-in) from loading or working.

## Plugin directories

Plugins live under a base directory, in a subdirectory per kind:

- Default base: `~/.config/skm/plugins/`
- Additional bases: set `SKM_PLUGINS_DIR` to one or more OS-path-separated directories.
- Provider plugins go in `<base>/providers/`.
- Target plugins go in `<base>/targets/`.

## Provider plugins

A **Provider** is where skm acquires assets from. Two built-ins ship with skm — Local and
GitHub — but you can add your own by dropping an executable into a `providers/` plugin
directory. skm discovers it, lists it, validates it, and routes imports/updates to it like
any built-in.

1. Build (or copy) an executable implementing the protocol below.
2. Make it executable (`chmod +x`) and place it under `<plugin dir>/providers/`.
3. Confirm it loaded: `skm provider list --json` (or the human table without `--json`).
4. If it didn't load, `skm provider list`/`validate` shows the specific reason — see
   [Isolation & diagnostics](#isolation--diagnostics) below.

## Protocol

One JSON object per request on **stdin**, one JSON object per response on **stdout**.
Anything you want to log goes to **stderr** — stdout must contain only the response.
Every call skm makes to your plugin is bounded by a timeout (currently 15s); if you don't
respond in time, the call is treated as a failure and your plugin is isolated for that
call (repeated timeouts keep it out of the loaded set).

> **Shell-script gotcha**: skm does **not** append a trailing newline to the request.
> POSIX `read -r line` still populates `line` correctly at EOF-without-newline, but
> returns a non-zero exit status. If your script uses `set -e`, that non-zero status
> will kill it immediately after the read, before your logic runs. Don't use `set -e`
> in a shell-script plugin (see the reference template).

### Required actions

**`id`** — return your provider's stable, unique identifier.

```jsonc
// request:  {"action":"id"}
// response: {"id":"acme"}
```

An empty or missing `id` is rejected — your plugin will fail to load.

**`can_handle`** — report whether you can fetch the given address.

```jsonc
// request:  {"action":"can_handle","address":"acme://org/repo"}
// response: {"result":true}
```

**`fetch`** — retrieve the asset at `address` into a fresh temp directory you create, and
return its path. skm copies out of it and owns cleanup afterward.

```jsonc
// request:  {"action":"fetch","address":"acme://org/repo"}
// response: {"path":"/tmp/your-staged-dir"}
// on failure:
// response: {"error":{"code":"fetch_failed","message":"clone failed: connection refused"}}
```

Error `code` should be one of: `unsupported_address`, `normalize_failed`, `fetch_failed`,
`protocol_error`, `timeout`. A bare string (`{"error":"message"}`) is still accepted for
backward compatibility and is treated as `fetch_failed`, but new plugins should use the
`{code,message}` form so skm can show a more specific reason.

### Optional actions

These make your plugin easier to discover and its addresses easier to reason about. A
plugin that doesn't implement one is treated as if it had answered with nothing — no
error, no isolation.

**`label`** — a human-readable name (falls back to your `id` if omitted).

```jsonc
// request:  {"action":"label"}
// response: {"label":"Acme Source"}
```

**`capability`** — describe what you handle, without fetching anything. Shown by
`skm provider list` so a user can tell what your provider is for before using it.

```jsonc
// request:  {"action":"capability"}
// response: {"description":"Fetches skills from acme://<org>/<repo>","schemes":["acme:"]}
```

**`normalize`** — canonicalize an address (e.g. strip a scheme, resolve to a canonical git
URL) for matching, fetch, and provenance. Return the address unchanged if you have nothing
to canonicalize.

```jsonc
// request:  {"action":"normalize","address":"acme://Org/Repo"}
// response: {"address":"acme://org/repo"}
```

## Isolation & diagnostics

At load time, a plugin is isolated (excluded from the loaded set, but recorded with a
reason, never fatal) when it:

- fails to launch or exits non-zero,
- returns an empty `id`,
- duplicates an `id` another already-loaded provider registered (the first one wins),
- returns invalid JSON or otherwise violates the protocol,
- times out.

Run `skm provider list --json` or `skm provider validate --json` to see every provider —
loaded or not — with its specific reason when it isn't.

## Reference template

See [`template/`](./template/) for a minimal, working shell-script plugin implementing
every action above. Copy it, rename it, and replace the logic in each `case` branch.

## Built-in git Providers (GitLab, Skills.sh)

Two extra sources ship as **built-in** Providers — no plugin needed:

| Provider id | Address form | Resolves to |
|---|---|---|
| `gitlab` | `owner/repo` or a git URL | `https://gitlab.com/<org>/<repo>` (shorthand) |
| `skills-sh` | `skills.sh://<owner>/<repo>` | `https://$SKM_SKILLS_SH_HOST/<owner>/<repo>.git` |

`gitlab` shares GitHub's exact access pattern — git URLs (`git@`/`https`/`ssh`/`.git`)
plus the `owner/repo` shorthand, resolved to its own https host — only the host
differs (`gitlab.com`). `skills-sh` covers the [skills.sh](https://skills.sh/)
directory, which indexes skills living in public GitHub repos, so its scheme
resolves to `github.com` by default; override with `SKM_SKILLS_SH_HOST` before
running skm.

## Target plugins

A **Target** is where skm installs assets to. Three built-in install strategies ship with
skm — `skill-symlink`, `command-marker`, `command-adapter` — but a tool with a different
on-disk convention (including a private/internal tool that has no place in skm's own
built-ins) is supported by dropping an executable into a `targets/` plugin directory and
referencing it from `targets.json` as strategy `plugin:<id>`.

```bash
skm target add --name my-tool --path ~/.my-tool/skills \
  --accepts skill --strategy skill=plugin:my-tool
```

1. Build (or copy) an executable implementing the protocol below.
2. Make it executable (`chmod +x`) and place it under `<plugin dir>/targets/`.
3. Confirm it loaded: `skm target plugin list --json` (or the human table without
   `--json`).
4. Point a target's strategy at it: `--strategy <kind>=plugin:<id>` on `target add`/
   `target update`, or select it from the strategy picker in the TUI target editor.
5. If it didn't load, or a target references it incoherently, `skm target validate`
   reports the specific reason.

### Protocol

Same shape as the Provider protocol: one JSON object per request on **stdin**, one JSON
object per response on **stdout**, diagnostics on **stderr**, a 15s per-call timeout, and
the same shell-script `set -e` gotcha described above.

Unlike a Provider (which stages content for skm to adopt), a Target plugin performs its
**own** filesystem I/O to place (or remove) the asset — it is trusted the same way a
Provider's `fetch` is; skm does not wrap its writes in a transaction.

#### Required actions

**`id`** — return your plugin's stable, unique identifier (the id used in `plugin:<id>`).

```jsonc
// request:  {"action":"id"}
// response: {"id":"my-tool"}
```

An empty or missing `id` is rejected — your plugin will fail to load.

**`install`** — place `source_path` (the repo entry) into `target_path` however your tool
expects, and report whether anything changed. `force` is set when the user passed
`--force`; if something incompatible already occupies your install location and `force` is
not set, return an error instead of overwriting it.

```jsonc
// request:  {"action":"install","name":"demo","kind":"skill",
//            "source_path":"/repo/skills/local/demo","target_path":"/home/u/.my-tool/skills","force":false}
// response: {"result":true}
// on failure:
// response: {"error":{"code":"install_failed","message":"…already exists; pass --force"}}
```

**`uninstall`** — remove your managed install of `name`/`kind` from `target_path`, and
report whether anything changed. Never remove something you didn't install.

```jsonc
// request:  {"action":"uninstall","name":"demo","kind":"skill","target_path":"/home/u/.my-tool/skills"}
// response: {"result":true}
```

**`state`** — classify the install health of `name`/`kind` within `target_path`.

```jsonc
// request:  {"action":"state","name":"demo","kind":"skill",
//            "source_path":"/repo/skills/local/demo","target_path":"/home/u/.my-tool/skills"}
// response: {"state":"installed"}
```

`state` is one of `absent`, `installed`, `conflict`, `dangling` — the same vocabulary skm's
built-in strategies use.

Error `code` should be one of: `install_failed`, `uninstall_failed`, `state_failed`,
`protocol_error`, `timeout`. A bare string (`{"error":"message"}`) is accepted for
consistency with Provider plugins and treated as `install_failed`.

#### Optional actions

**`label`** — a human-readable name (falls back to your `id` if omitted).

```jsonc
// request:  {"action":"label"}
// response: {"label":"My Tool"}
```

**`capability`** — describe what you handle, without installing anything. Shown by
`skm target plugin list`, and used by `target validate`/the TUI to check a target's
strategy actually matches a kind you support.

```jsonc
// request:  {"action":"capability"}
// response: {"description":"Installs into My Tool's own skills layout","kinds":["skill"]}
```

Omitting `kinds` (or answering with an empty list) is treated as "supports any kind" —
the same permissive fallback a Provider's missing `capability` gets.

### Isolation & diagnostics

Same rules as Provider plugins: a plugin that fails to launch, returns an empty `id`,
duplicates an `id` another loaded Target plugin registered, violates the protocol, or
times out is isolated (excluded, recorded with a reason, never fatal). Run
`skm target plugin list --json` to see every Target plugin — loaded or not — with its
specific reason when it isn't; run `skm target validate` to see whether a target's
declared `plugin:<id>` strategy actually resolves and matches its accepted kind.
