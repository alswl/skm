# skm Provider plugins

A **Provider** is where skm acquires assets from. Two built-ins ship with skm — Local and
GitHub — but you can add your own **without modifying or rebuilding skm** by dropping an
executable into a plugin directory. skm discovers it, lists it, validates it, and routes
imports/updates to it like any built-in.

## Installing a plugin

1. Build (or copy) an executable implementing the protocol below.
2. Make it executable (`chmod +x`) and place it in a plugin directory:
   - Default: `~/.config/skm/plugins/`
   - Additional dirs: set `SKM_PLUGINS_DIR` to one or more OS-path-separated directories.
3. Confirm it loaded: `skm provider list --json` (or the human table without `--json`).
4. If it didn't load, `skm provider list`/`validate` shows the specific reason — see
   [Isolation & diagnostics](#isolation--diagnostics) below.

A broken, slow, or hung plugin is **isolated**: it never crashes skm and never prevents
other providers (built-in or plugin) from loading or working.

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
