#!/usr/bin/env bash
# Bump project version using semtag. Optionally generates CHANGELOG via git-cliff.
# github.com/alswl/makefile-go
#
# Author: alswl
# Version: 0.5.0
#
# Usage: hack/bump.sh [--stage <stage>] [--scope <scope>] [--dry-run <dry-run>] [--push <push>] [--post-actions-script <post-actions.sh>]
#   --stage:    final, alpha, beta, candidate (default: final)
#   --scope:    major, minor, patch (default: major)
#   --dry-run:  true|false (default: true)
#   --push:     true|false (default: false)
#   --post-actions-script: path to script run after VERSION write

pushd "$(dirname "$0")/.." > /dev/null
set -e

bump_stage="final"
bump_scope="major"
bump_dry_run="true"
bump_push="false"
post_actions_script=""

has_git_cliff() {
  command -v git-cliff >/dev/null 2>&1
}

function runPostActions {
  if [ -n "$post_actions_script" ]; then
    if [ ! -f "$post_actions_script" ]; then
      echo "post actions script not found: $post_actions_script"
    fi
    $post_actions_script
  fi
}

while [ $# -gt 0 ]; do
  case "$1" in
    --stage)
      bump_stage=$2
      shift
      ;;
    --scope)
      bump_scope=$2
      shift
      ;;
    --dry-run)
      bump_dry_run=$2
      shift
      ;;
    --push)
      bump_push=$2
      shift
      ;;
    --post-actions-script)
      post_actions_script=$2
      shift
      ;;
    *)
      echo "unknown option: $1"
      exit 1
      ;;
  esac
  shift
done

# Auto-detect post-actions script if not explicitly specified
if [ -z "$post_actions_script" ] && [ -f "./hack/bump-post-actions.sh" ]; then
  post_actions_script="./hack/bump-post-actions.sh"
fi

next=$(semtag "$bump_stage" -s "$bump_scope" -f -o)
current=$(cat VERSION 2>/dev/null || echo "unknown")

if [ "$bump_dry_run" = "true" ]; then
  echo ""
  echo "=============================="
  echo "  Version Bump Preview"
  echo "=============================="
  echo "  Current version : $current"
  echo "  Next version    : $next"
  echo "  Scope           : $bump_scope"
  echo "  Stage           : $bump_stage"
  echo "------------------------------"
  echo "  Actions (if DRY_RUN=false):"
  echo "    1. Write $next to VERSION"
  if has_git_cliff; then
    echo "    2. Generate CHANGELOG.md (via git cliff)"
  fi
  echo "    3. git commit \"chore: Bump version to $next\""
  echo "    4. git tag (via semtag $bump_stage -s $bump_scope)"
  if [ -n "$post_actions_script" ]; then
    echo "    *. Run post-actions: $post_actions_script"
  fi
  echo "    5. Write ${next}-dev to VERSION"
  echo "    6. git commit \"chore: prepare next version ${next}-dev\""
  echo "=============================="
  if has_git_cliff; then
    echo ""
    echo "------------------------------"
    echo "  Changelog Preview (--latest)"
    echo "------------------------------"
    git cliff -t "$next" --latest 2>/dev/null || echo "  (git cliff failed, skipped)"
    echo "=============================="
  else
    echo "  (git-cliff not installed, changelog preview skipped)"
  fi
  echo ""
  echo "  To execute, re-run with: DRY_RUN=false"
  echo ""
  exit 0
fi

# release version
echo "${next}" > VERSION
runPostActions

if has_git_cliff; then
  echo "Generating CHANGELOG.md..."
  git cliff -t "$next" -o CHANGELOG.md
else
  echo "(git-cliff not installed, skipping CHANGELOG generation)"
fi

git add .
git commit -m "chore: Bump version to $next"
semtag "$bump_stage" -s "$bump_scope"

# dev version
echo "${next}-dev" > VERSION
runPostActions
git add .
git commit -m "chore: prepare next version $next-dev"

# push
tag=$(git describe --tags --abbrev=0)

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
