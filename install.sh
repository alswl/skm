#!/bin/sh
# Install skm from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/alswl/skm/master/install.sh | sh
#
# Optional environment variables:
#   SKM_VERSION      install a specific released version instead of the latest (e.g. v0.1.1)
#   SKM_INSTALL_DIR  install directory (default: first writable dir in /opt/homebrew/bin,
#                    ~/.local/bin, /usr/local/bin, ~/bin)
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

# Pick the install directory.
if [ -n "${SKM_INSTALL_DIR:-}" ]; then
	INSTALL_DIR="$SKM_INSTALL_DIR"
else
	INSTALL_DIR=""
	for d in /opt/homebrew/bin "$HOME/.local/bin" /usr/local/bin "$HOME/bin"; do
		if [ -d "$d" ] && [ -w "$d" ]; then
			INSTALL_DIR="$d"
			break
		fi
	done
fi
if [ -z "$INSTALL_DIR" ]; then
	die "No writable install directory found. Pick one explicitly, e.g.
    SKM_INSTALL_DIR=\"\$HOME/.local/bin\" sh -c 'curl -fsSL ${INSTALL_SCRIPT_URL} | sh'
  or, for a system-wide install:
    curl -fsSL ${INSTALL_SCRIPT_URL} -o /tmp/skm-install.sh && sudo SKM_INSTALL_DIR=/usr/local/bin sh /tmp/skm-install.sh"
fi
mkdir -p "$INSTALL_DIR"
[ -w "$INSTALL_DIR" ] || die "Install directory is not writable: ${INSTALL_DIR}. Set SKM_INSTALL_DIR to a writable directory."

install -m 0755 "$tmpdir/${PROJECT}-${OS}-${ARCH}" "$INSTALL_DIR/$PROJECT"
log "Installed ${PROJECT} ${VERSION} -> ${INSTALL_DIR}/${PROJECT}"

if [ -x "$INSTALL_DIR/$PROJECT" ]; then
	"$INSTALL_DIR/$PROJECT" version
fi

case ":${PATH}:" in
	*":${INSTALL_DIR}:"*) ;;
	*) warn "${INSTALL_DIR} is not on your PATH. Add: export PATH=\"${INSTALL_DIR}:\$PATH\"";;
esac

existing="$(command -v "$PROJECT" 2>/dev/null || true)"
if [ -n "$existing" ] && [ "$existing" != "$INSTALL_DIR/$PROJECT" ]; then
	warn "'${PROJECT}' currently resolves to '${existing}', which shadows the newly installed binary."
fi
