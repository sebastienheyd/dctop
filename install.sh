#!/bin/sh
# Install the latest dctop release.
#
#   curl -fsSL https://raw.githubusercontent.com/sebastienheyd/dctop/main/install.sh | sh
#
# The binary goes to $DCTOP_INSTALL_DIR, by default ~/.local/bin.
set -eu

repo="sebastienheyd/dctop"
install_dir="${DCTOP_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "dctop install: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need curl
need tar
need uname

case "$(uname -s)" in
Linux) os=linux ;;
Darwin) os=darwin ;;
*) fail "unsupported operating system: $(uname -s) (Linux and macOS only)" ;;
esac

case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported architecture: $(uname -m) (amd64 and arm64 only)" ;;
esac

tag=$(curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
	sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
[ -n "$tag" ] || fail "cannot find the latest release of $repo"
version=${tag#v}

archive="dctop_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$repo/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading dctop $version ($os/$arch)..."
curl -fsSL -o "$tmp/$archive" "$base/$archive" || fail "cannot download $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "cannot download checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | cut -d ' ' -f 1)
[ -n "$expected" ] || fail "no checksum for $archive"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$archive" | cut -d ' ' -f 1)
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "$tmp/$archive" | cut -d ' ' -f 1)
else
	fail "sha256sum or shasum is required to verify the download"
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" dctop
mkdir -p "$install_dir"
mv "$tmp/dctop" "$install_dir/dctop"
chmod 755 "$install_dir/dctop"
echo "Installed $install_dir/dctop"

case ":$PATH:" in
*":$install_dir:"*) ;;
*) echo "Warning: $install_dir is not in your PATH. Add it to run dctop from anywhere." >&2 ;;
esac
