#!/usr/bin/env python3
"""Check that every leaf in skm's public help has a review disposition."""

import argparse
from html import escape
import json
from pathlib import Path
import re
import subprocess
import sys

COMMAND = re.compile(r"[a-z][a-z0-9-]*(?: [a-z][a-z0-9-]*)*")
HELP_ROW = re.compile(r"^  ([a-z][a-z0-9-]*)(?:[ \t]+.*)?$")
STATUSES = {
    "existing": "已有 TUI",
    "partial": "部分 TUI",
    "proposed": "拟议 TUI",
    "cli-only": "CLI 专用",
}
DEFAULT_MAP = Path(__file__).resolve().parents[1] / "docs/tui-cli-coverage.json"


def public_leaves(binary, parts=()):
    output = subprocess.run(
        [str(binary), *parts, "--help"], check=True, capture_output=True,
        text=True, timeout=10,
    ).stdout.splitlines()
    if "Available Commands:" not in output:
        if not parts:
            raise ValueError("root help has no Available Commands section")
        return {" ".join(parts)}
    start = output.index("Available Commands:") + 1
    children = []
    for line in output[start:]:
        if not line.strip():
            break
        match = HELP_ROW.fullmatch(line)
        if not match:
            raise ValueError(f"{' '.join(parts) or 'skm'}: unexpected help row {line!r}")
        children.append(match[1])
    if not children or len(children) != len(set(children)):
        raise ValueError(f"{' '.join(parts) or 'skm'}: invalid help command list")
    leaves = set()
    for child in children:
        leaves.update(public_leaves(binary, (*parts, child)))
    return leaves


def check(data, leaves):
    pages = data["pages"]
    seen = set()
    errors = []
    for row in data["commands"]:
        command = row["command"]
        if not isinstance(command, str) or not COMMAND.fullmatch(command):
            raise ValueError(f"invalid command path {command!r}")
        if command in seen:
            errors.append(f"{command}: duplicate mapping")
        seen.add(command)
        status = row.get("status")
        if status not in STATUSES:
            errors.append(f"{command}: choose a valid status")
        elif status == "cli-only":
            if not isinstance(row.get("note"), str) or not row["note"].strip():
                errors.append(f"{command}: give a reason for the cli-only disposition")
        if status != "cli-only" or "page" in row:
            if row.get("page") not in pages:
                errors.append(f"{command}: reference a registered Page ID")
    errors.extend(f"{name}: add a Page ID or a cli-only reason" for name in sorted(leaves - seen))
    errors.extend(f"{name}: remove or rename the stale mapping" for name in sorted(seen - leaves))
    return errors


def render(data):
    yield '<table aria-label="CLI 能力对照">'
    yield '<thead><tr><th>命令</th><th>覆盖及 Page</th><th>说明</th></tr></thead><tbody>'
    for row in data["commands"]:
        page = row.get("page")
        status = escape(STATUSES[row["status"]])
        if page is not None:
            status += f' · <a href="#screen-{escape(page)}">{escape(data["pages"][page])}</a>'
        command = escape(row["command"])
        note = escape(row.get("note", ""))
        yield f'<tr><th scope="row"><code>skm {command}</code></th><td>{status}</td><td>{note}</td></tr>'
    yield '</tbody></table>'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--mapping", type=Path, default=DEFAULT_MAP)
    parser.add_argument("--html", action="store_true", help="emit the checked review table fragment")
    args = parser.parse_args()
    try:
        data = json.loads(args.mapping.read_text(encoding="utf-8"))
        leaves = public_leaves(args.binary.resolve())
        errors = check(data, leaves)
        if not errors:
            if args.html:
                print("\n".join(render(data)))
            else:
                print(f"skm-cli-coverage: {len(leaves)} public leaf commands mapped")
            return 0
    except (OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.SubprocessError) as error:
        errors = [str(error)]
    for error in errors:
        print(f"skm-cli-coverage: {args.mapping}: {error}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
