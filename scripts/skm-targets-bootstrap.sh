#!/usr/bin/env bash
# Provision the conventional skill and command targets without a target plugin.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: skm-targets-bootstrap.sh <base-name> <skills-directory> [--config <config-directory>]
EOF
}

if [ "${1:-}" = "--help" ] || [ "${1:-}" = "-h" ]; then usage; exit 0; fi
if [ "$#" -lt 2 ]; then usage >&2; exit 2; fi
base=$1
directory=$2
shift 2
config_args=()
if [ "$#" -gt 0 ]; then
  if [ "$#" -ne 2 ] || [ "$1" != "--config" ] || [ -z "$2" ]; then usage >&2; exit 2; fi
  config_args=(--config "$2")
fi
# An option token in a positional slot means the positional is missing: without
# this, `--config <dir>` alone parses as base=--config, directory=<dir>, and the
# swallowed --config leaves the writes going to the default config directory.
case "$base" in
  -*) echo "missing <base-name>: got option $base" >&2; usage >&2; exit 2;;
esac
case "$directory" in
  -*) echo "missing <skills-directory>: got option $directory" >&2; usage >&2; exit 2;;
  '') usage >&2; exit 2;;
esac
case "$base" in
  *[!A-Za-z0-9._-]*|'') echo "invalid base name: $base" >&2; exit 2;;
esac
command -v skm >/dev/null 2>&1 || { echo "skm is required on PATH" >&2; exit 2; }
command -v python3 >/dev/null 2>&1 || { echo "python3 is required on PATH" >&2; exit 2; }

directory=$(python3 - "$directory" <<'PY'
import os, sys
print(os.path.abspath(os.path.expanduser(sys.argv[1])))
PY
)
mkdir -p "$directory"

list_targets() { skm target list --json "${config_args[@]}"; }
initial=$(list_targets) || { echo "unable to list targets" >&2; exit 1; }
actions=$(python3 - "$initial" "$base" "$directory" <<'PY'
import json, sys
try:
    report=json.loads(sys.argv[1])
    targets=report['targets']
    invalid=report.get('invalid', [])
    if invalid or not isinstance(targets, list): raise ValueError('unreadable target entries')
except Exception as e:
    raise SystemExit('invalid target list JSON: %s' % e)
base, path=sys.argv[2:]
desired=[
 {'name':base,'platform':base,'path':path,'accepts':['skill'],'strategies':{'skill':'skill-symlink'}},
 {'name':base+'-commands','platform':base,'path':path,'accepts':['command'],'strategies':{'command':'command-adapter'}},
]
by_name={t.get('name'):t for t in targets}
for d in desired:
    old=by_name.get(d['name'])
    if old and old.get('builtin'):
        raise SystemExit('target %s is built in; choose another base name' % d['name'])
    if not old: action='add'
    elif (old.get('platform') == d['platform'] and old.get('path') == d['path'] and
          sorted(old.get('accepts', [])) == d['accepts'] and old.get('strategies') == d['strategies']): action='unchanged'
    else: action='update'
    print('%s\t%s\t%s\t%s\t%s' % (action,d['name'],d['platform'],d['accepts'][0],next(iter(d['strategies'].values()))))
PY
) || { echo "$actions" >&2; exit 1; }

while IFS=$'\t' read -r action name platform accepts strategy; do
  [ -n "$action" ] || continue
  case "$action" in
    unchanged) echo "unchanged $name";;
    add|update)
      if ! skm target "$action" --name "$name" --platform "$platform" --path "$directory" --accepts "$accepts" --strategy "$accepts=$strategy" "${config_args[@]}" >/dev/null; then
        echo "$action failed for $name; completed target changes remain in place" >&2
        exit 1
      fi
      case "$action" in add) echo "added $name";; update) echo "updated $name";; esac;;
  esac
done <<EOF
$actions
EOF

final=$(list_targets) || { echo "final target listing failed; completed target changes remain in place" >&2; exit 1; }
python3 - "$final" "$base" "$directory" <<'PY'
import json, sys
try:
    report=json.loads(sys.argv[1]); targets=report['targets']
    if report.get('invalid'): raise ValueError('invalid target entries')
except Exception as e: raise SystemExit('final target listing is invalid: %s' % e)
base,path=sys.argv[2:]
expected={base:(base,['skill'],{'skill':'skill-symlink'}), base+'-commands':(base,['command'],{'command':'command-adapter'})}
actual={t.get('name'):t for t in targets}
for name,(platform,accepts,strategies) in expected.items():
    t=actual.get(name)
    if not t or t.get('platform')!=platform or t.get('path')!=path or sorted(t.get('accepts',[]))!=accepts or t.get('strategies')!=strategies:
        raise SystemExit('final target listing does not match %s' % name)
PY
