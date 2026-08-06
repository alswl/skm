#!/usr/bin/env bash
# Bump sub-module version. Auto-detects scope vs direct version for backward compatibility.
# github.com/alswl/makefile-go
#
# Author: alswl
# Version: 0.3.0
#
# Usage: hack/bump-sub-mod.sh <sub> <scope|version> [dryrun] [push]
#   sub:     sub-module directory (required)
#   scope:   major, minor, patch (auto-calculate next version)
#   version: v1.2.3 format (use directly, backward-compatible)
#   dryrun:  true|false (default: true)
#   push:    true|false (default: false)
#
# Examples:
#   hack/bump-sub-mod.sh mymod patch true          # dry-run, auto-calc next patch
#   hack/bump-sub-mod.sh mymod minor false          # execute, auto-calc next minor
#   hack/bump-sub-mod.sh mymod v1.2.3 false         # execute, use specified version (legacy)

pushd "$(dirname "$0")/.." > /dev/null

set -e

sub=$1
second_arg=${2:-patch}
bump_dry_run=${3:-true}
bump_push=${4:-false}

if [ -z "$sub" ]; then
  echo "sub mod is required"
  exit 1
fi
if [ ! -d "$sub" ]; then
  echo "sub mod $sub does not exist"
  exit 1
fi

# auto-detect: v?X.Y.Z → legacy mode, major|minor|patch → scope mode
next=""
bump_scope=""
if echo "$second_arg" | grep -qE '^v?[0-9]+\.[0-9]+\.[0-9]+'; then
  next="$second_arg"
  [[ "$next" != v* ]] && next="v$next"
else
  bump_scope="$second_arg"
fi

if [ -z "$next" ]; then
  latest_tag=$(git tag -l "${sub}/v*" --sort=-v:refname | head -n 1)

  if [ -z "$latest_tag" ]; then
    latest_version="0.0.0"
  else
    latest_version=${latest_tag#"${sub}/v"}
  fi

  IFS='.' read -r major minor patch <<< "${latest_version%%[-+]*}"
  major=${major:-0}
  minor=${minor:-0}
  patch=${patch:-0}

  case "$bump_scope" in
    major) next="v$((major + 1)).0.0" ;;
    minor) next="v${major}.$((minor + 1)).0" ;;
    patch) next="v${major}.${minor}.$((patch + 1))" ;;
    *) echo "Invalid scope or version: $bump_scope (expected: major|minor|patch or vX.Y.Z)"; exit 1 ;;
  esac
fi

current=$(cat "${sub}/VERSION" 2>/dev/null || echo "unknown")

if [ "$bump_dry_run" = "true" ]; then
  echo ""
  echo "=============================="
  echo "  Sub-Module Bump Preview"
  echo "=============================="
  echo "  Sub-module      : $sub"
  echo "  Current version : $current"
  echo "  Next version    : $next"
  if [ -n "$bump_scope" ]; then
    echo "  Scope           : $bump_scope"
  else
    echo "  Mode            : direct version (legacy)"
  fi
  echo "------------------------------"
  echo "  Actions (if DRY_RUN=false):"
  echo "    1. Write $next to ${sub}/VERSION"
  echo "    2. git commit \"chore: Bump version to $next\""
  echo "    3. git tag ${sub}/${next}"
  echo "    4. Write ${next}-dev to ${sub}/VERSION"
  echo "    5. git commit \"chore: prepare next version ${next}-dev\""
  echo "=============================="
  echo ""
  echo "  To execute, re-run with: DRY_RUN=false"
  echo ""
  exit 0
fi

# release version
echo "${next}" > "${sub}"/VERSION
git add "${sub}"/VERSION
git commit -m "chore: Bump version to $next"

tag="${sub}/${next}"
git tag "$tag"

# dev version
echo "${next}-dev" > "${sub}"/VERSION
git add "${sub}"/VERSION
git commit -m "chore: prepare next version ${next}-dev"

# push
echo ""
echo "=============================="
echo "  Release Complete"
echo "=============================="
echo "  Tag: $tag"
echo ""
echo "  Push commands:"
echo "    git push origin $tag"
echo "    git push"
echo "=============================="

if [ "$bump_push" = "true" ]; then
  echo ""
  echo "Pushing..."
  git push origin "$tag"
  git push
  echo "Done."
elif [ -t 0 ]; then
  echo ""
  read -p "Push now? (y/N) " answer
  if [ "$answer" = "y" ] || [ "$answer" = "Y" ]; then
    git push origin "$tag"
    git push
    echo "Done."
  fi
fi

popd > /dev/null
