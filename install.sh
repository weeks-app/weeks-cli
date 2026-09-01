#!/bin/sh
set -eu

repo="weeks-app/weeks-cli"
bin_name="weeks"

say() {
  printf '%s\n' "$*"
}

fail() {
  say "weeks install: $*" >&2
  exit 1
}

need() {
  command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need curl
need tar

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin|linux) ;;
  *) fail "unsupported operating system: $os" ;;
esac

case "$arch" in
  x86_64|amd64) arch="x86_64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) fail "unsupported architecture: $arch" ;;
esac

version="${WEEKS_INSTALL_VERSION:-latest}"
install_dir="${WEEKS_INSTALL_DIR:-$HOME/.local/bin}"
tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

if [ "$version" = "latest" ]; then
  need sed
  version="$(
    curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
      sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' |
      head -n 1
  )"
  [ -n "$version" ] || fail "could not resolve the latest release"
fi

case "$version" in
  v*) ;;
  *) version="v$version" ;;
esac

plain_version="${version#v}"
asset="${bin_name}_${plain_version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$repo/releases/download/$version"

say "Downloading $bin_name $version for $os/$arch"
curl -fsSLo "$tmp_dir/$asset" "$base_url/$asset"
curl -fsSLo "$tmp_dir/checksums.txt" "$base_url/checksums.txt"

if command -v sha256sum >/dev/null 2>&1; then
  checksum_cmd="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  checksum_cmd="shasum -a 256"
else
  fail "sha256sum or shasum is required"
fi

expected="$(grep "  $asset\$" "$tmp_dir/checksums.txt" | awk '{print $1}')"
[ -n "$expected" ] || fail "checksum for $asset not found"
actual="$($checksum_cmd "$tmp_dir/$asset" | awk '{print $1}')"
[ "$actual" = "$expected" ] || fail "checksum mismatch"

mkdir -p "$install_dir"
tar -xzf "$tmp_dir/$asset" -C "$tmp_dir" "$bin_name"
install "$tmp_dir/$bin_name" "$install_dir/$bin_name"

if [ "$os" = "darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$install_dir/$bin_name" >/dev/null 2>&1 || true
fi

installed_version="$("$install_dir/$bin_name" version --json 2>/dev/null | sed -n 's/.*"version":[[:space:]]*"\([^"]*\)".*/weeks \1/p' | head -n 1)"
[ -n "$installed_version" ] || installed_version="$bin_name $version"
say "Installed $installed_version"
say "Binary: $install_dir/$bin_name"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) say "Add $install_dir to PATH to run $bin_name from any shell." ;;
esac
