#!/usr/bin/env bash
# Generate CHANGELOG.md using git-cliff with the correct next version tag.
# github.com/alswl/makefile-go
#
# Author: alswl
# Version: 0.1.0
#
# Usage: hack/gen-changelog.sh --stage <stage> --scope <scope>
#   --stage:  final, alpha, beta, candidate (default: final)
#   --scope:  major, minor, patch (default: minor)

pushd "$(dirname "$0")/.." > /dev/null

stage="final"
scope="minor"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --stage) stage="$2"; shift 2 ;;
    --scope) scope="$2"; shift 2 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

if ! command -v git-cliff >/dev/null 2>&1; then
  echo "git-cliff is not installed. Install: brew install git-cliff"
  exit 1
fi

next=$(semtag "$stage" -s "$scope" -f -o 2>/dev/null || echo "unknown")
echo "Generating CHANGELOG.md for version: $next"
git cliff -t "$next" -o CHANGELOG.md

popd > /dev/null
