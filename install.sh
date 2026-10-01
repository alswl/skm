#!/bin/sh
# Install skm from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/alswl/skm/master/install.sh | sh
#
# Optional environment variables:
#   SKM_VERSION      install a specific released version instead of the latest (e.g. v0.1.1)
#   SKM_INSTALL_DIR  install directory (default: $XDG_BIN_HOME, else ~/.local/bin)
#
# Supported platforms match the release matrix: macOS/Linux on amd64/arm64.
set -eu

REPO="alswl/skm"
PROJECT="skm"
BASE_URL="https://github.com/${REPO}"
INSTALL_SCRIPT_URL="https://raw.githubusercontent.com/${REPO}/master/install.sh"

log() { printf '==> %s\n' "$*"; }
warn() { printf '==> %s\n' "$*" >&2; }
die() { printf 'Error: %s\n' "$*" >&2; exit 1; }

# Resolve the version: SKM_VERSION wins, otherwise follow the releases/latest
# redirect (no GitHub API, so no rate limits). When no stable release has been
# published (e.g. the newest release is a pre-release), fall back to the
# newest release from the API.
if [ -n "${SKM_VERSION:-}" ]; then
	VERSION="$SKM_VERSION"
else
	latest_url="${BASE_URL}/releases/latest"
	VERSION=""
	if VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$latest_url" 2>/dev/null)"; then
		case "$VERSION" in
			*/releases/tag/v*) VERSION="${VERSION##*/}" ;;
			*) VERSION="" ;;
		esac
	else
		# curl may print url_effective and still fail (flaky network); a
		# failed probe must not leave a stale, non-empty VERSION behind.
		VERSION=""
	fi
	if [ -z "$VERSION" ]; then
		log "No stable release found at ${latest_url}; falling back to the newest release."
		api_url="https://api.github.com/repos/${REPO}/releases?per_page=1"
		VERSION="$(curl -fsSL "$api_url" 2>/dev/null | awk -F'"' '/"tag_name"/ {print $4; exit}')" || VERSION=""
	fi
fi
case "$VERSION" in
	v*) ;;
	*) die "Could not determine the latest release version. Pin one explicitly, e.g. SKM_VERSION=v0.1.1 sh -c 'curl -fsSL ${INSTALL_SCRIPT_URL} | sh'";;
esac

# Map the host OS/architecture to the release matrix naming.
OS="$(uname -s)"
case "$OS" in
	Darwin) OS="darwin" ;;
	Linux) OS="linux" ;;
	*) die "Unsupported OS '${OS}'. skm releases only cover macOS and Linux.";;
esac

ARCH="$(uname -m)"
case "$ARCH" in
	x86_64|amd64) ARCH="amd64" ;;
	aarch64|arm64) ARCH="arm64" ;;
	*) die "Unsupported architecture '${ARCH}'. skm releases only cover amd64 and arm64.";;
esac

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

asset="${PROJECT}-${VERSION}-${OS}-${ARCH}.tar.gz"
url="${BASE_URL}/releases/download/${VERSION}/${asset}"

log "Downloading ${asset}"
# Retry on transient failures and resume a partial download where possible.
curl -fsSL --retry 3 --retry-delay 2 -C - "$url" -o "$tmpdir/$asset" \
	|| die "Failed to download ${url}. Does the release exist? Try SKM_VERSION=vX.Y.Z."

sha256_hex() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" 2>/dev/null | awk '{print $1}'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" 2>/dev/null | awk '{print $1}'
	else
		return 1
	fi
}

# Verify the archive against the checksums.txt published with the release,
# when one exists.
verify_checksum() {
	checksums_url="${BASE_URL}/releases/download/${VERSION}/checksums.txt"
	if ! curl -fsSL "$checksums_url" -o "$tmpdir/checksums.txt" 2>/dev/null; then
		warn "No checksums.txt for ${VERSION}; skipping checksum verification."
		return 0
	fi
	expected="$(awk -v a="$asset" '$2 == a {print $1}' "$tmpdir/checksums.txt")"
	if [ -z "$expected" ]; then
		warn "No checksum entry for ${asset}; skipping checksum verification."
		return 0
	fi
	actual="$(sha256_hex "$tmpdir/$asset")" || die "Neither sha256sum nor shasum is available to verify the checksum."
	[ "$actual" = "$expected" ] || die "Checksum mismatch for ${asset}: expected ${expected}, got ${actual}."
	log "Checksum verified for ${asset}"
}
verify_checksum

# The tarball holds the binary as skm-<os>-<arch> (see .github/workflows/release.yml).
tar -xzf "$tmpdir/$asset" -C "$tmpdir" "${PROJECT}-${OS}-${ARCH}" || die "Failed to extract ${asset}."

# Pick the install directory: a per-user bin dir, like uv, pipx and mise.
# Package-manager prefixes (/opt/homebrew/bin, /usr/local/bin) belong to
# their owners; an unmanaged binary there collides with brew link and needs
# sudo to replace.
INSTALL_DIR="${SKM_INSTALL_DIR:-${XDG_BIN_HOME:-$HOME/.local/bin}}"
mkdir -p "$INSTALL_DIR" || die "Cannot create install directory: ${INSTALL_DIR}. Set SKM_INSTALL_DIR to a writable directory."
[ -w "$INSTALL_DIR" ] || die "Install directory is not writable: ${INSTALL_DIR}. Set SKM_INSTALL_DIR to a writable directory, or for a system-wide install:
    curl -fsSL ${INSTALL_SCRIPT_URL} -o /tmp/skm-install.sh && sudo SKM_INSTALL_DIR=/usr/local/bin sh /tmp/skm-install.sh"

installed="$INSTALL_DIR/$PROJECT"
install -m 0755 "$tmpdir/${PROJECT}-${OS}-${ARCH}" "$installed"
log "Installed ${PROJECT} ${VERSION} -> ${installed}"
"$installed" version

# Report every other skm on PATH: a copy left by an older install (earlier
# versions of this script preferred /opt/homebrew/bin and /usr/local/bin), or
# an unrelated program of the same name (Homebrew's "skm" formula is an SSH
# key manager). Foreign binaries are identified by our module path rather than
# executed. Conflicts are only reported, never removed.
first=""
conflicts=0
seen=":"
old_ifs=$IFS
IFS=:
for d in $PATH; do
	[ -n "$d" ] || continue
	case "$seen" in *":$d:"*) continue ;; esac
	seen="$seen$d:"
	f="$d/$PROJECT"
	[ -f "$f" ] && [ -x "$f" ] || continue
	[ -n "$first" ] || first="$f"
	[ "$f" -ef "$installed" ] && continue
	conflicts=$((conflicts + 1))
	if LC_ALL=C grep -aq 'github.com/alswl/skm/skm' "$f" 2>/dev/null; then
		what="skm $("$f" version 2>/dev/null | awk '{print $2}')"
	else
		what="a different program named ${PROJECT}"
	fi
	case "$(readlink "$f" 2>/dev/null || true)" in
		*/Cellar/*) fix="brew uninstall ${PROJECT}" ;;
		*) fix="rm \"$f\"" ;;
	esac
	warn "Conflict: ${f} is ${what}. Remove it with: ${fix}"
done
IFS=$old_ifs

if [ -z "$first" ]; then
	warn "${INSTALL_DIR} is not on your PATH. Add to your shell profile: export PATH=\"${INSTALL_DIR}:\$PATH\""
elif ! [ "$first" -ef "$installed" ]; then
	warn "'${PROJECT}' resolves to ${first}, which shadows ${installed}. Remove the conflict above, then run 'hash -r'."
elif [ "$conflicts" -gt 0 ]; then
	warn "${installed} comes first on PATH; the conflicts above are shadowed but may confuse other shells or tools."
fi
